// mailserver-canary tests whether a mailserver enode responds to a historic messages request.
package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	stdlog "log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv6"
	"github.com/status-im/status-go/api"
	"github.com/status-im/status-go/logutils"
	"github.com/status-im/status-go/params"
	"github.com/status-im/status-go/rpc"
	"github.com/status-im/status-go/t/helpers"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	mailboxPassword = "status-offline-inbox"
)

// All general log messages in this package should be routed through this logger.
var logger = log.New("package", "status-go/cmd/mailserver-canary")

var (
	enodeAddr        = flag.String("mailserver", "", "mailserver enode address (e.g. enode://1da276e34126e93babf24ec88aac1a7602b4cbb2e11b0961d0ab5e989ca9c261aa7f7c1c85f15550a5f1e5a5ca2305b53b9280cf5894d5ecf7d257b173136d40@167.99.209.61:30504)")
	publicChannel    = flag.String("channel", "status", "The public channel name to retrieve historic messages from")
	period           = flag.Int("period", 24*60*60, "How far in the past to request messages from mailserver, in seconds")
	minPow           = flag.Float64("shh.pow", params.WhisperMinimumPoW, "PoW for messages to be added to queue, in float format")
	ttl              = flag.Int("shh.ttl", params.WhisperTTL, "Time to live for messages, in seconds")
	homePath         = flag.String("home-dir", ".", "Home directory where state is stored")
	logLevel         = flag.String("log", "INFO", `Log level, one of: "ERROR", "WARN", "INFO", "DEBUG", and "TRACE"`)
	logFile          = flag.String("logfile", "", "Path to the log file")
	logWithoutColors = flag.Bool("log-without-color", false, "Disables log colors")
)

func main() {
	exitCode := 1
	writeExitMessageFunc := func() { logger.Info("Mailserver responded correctly", "address", enodeAddr) }
	defer func() {
		writeExitMessageFunc()
		os.Exit(exitCode)
	}()

	flag.Parse()

	colors := !(*logWithoutColors)
	if colors {
		colors = terminal.IsTerminal(int(os.Stdin.Fd()))
	}

	if err := logutils.OverrideRootLog(*logLevel != "", *logLevel, *logFile, colors); err != nil {
		stdlog.Fatalf("Error initializing logger: %s", err)
	}

	if enodeAddr == nil {
		writeExitMessageFunc = func() { logger.Crit("No mailserver address specified", "enodeAddr", *enodeAddr) }
		return
	}

	mailserverParsedNode, err := discv5.ParseNode(*enodeAddr)
	if err != nil {
		writeExitMessageFunc = func() { logger.Crit("Invalid mailserver address specified", "enodeAddr", *enodeAddr, "error", err) }
		return
	}

	clientBackend, stopFunc, err := startClientNode()
	defer stopFunc()
	if err != nil {
		return
	}

	clientWhisperService, err := clientBackend.StatusNode().WhisperService()
	if err != nil {
		writeExitMessageFunc = func() { logger.Error("Could not retrieve Whisper service", "error", err) }
		return
	}

	// add mailserver peer to client
	clientErrCh := helpers.WaitForPeerAsync(clientBackend.StatusNode().Server(), *enodeAddr, p2p.PeerEventTypeAdd, 5*time.Second)
	err = clientBackend.StatusNode().AddPeer(*enodeAddr)
	if err != nil {
		writeExitMessageFunc = func() { logger.Error("Failed to add mailserver peer to client", "error", err) }
		return
	}

	err = <-clientErrCh
	if err != nil {
		writeExitMessageFunc = func() { logger.Error("Error detected while waiting for mailserver peer to be added", "error", err) }
		return
	}

	// add mailserver sym key
	mailServerKeyID, err := clientWhisperService.AddSymKeyFromPassword(mailboxPassword)
	if err != nil {
		writeExitMessageFunc = func() { logger.Error("Error adding mailserver sym key to client peer", "error", err) }
		return
	}

	mailboxPeer := mailserverParsedNode.ID[:]
	mailboxPeerStr := mailserverParsedNode.ID.String()
	err = clientWhisperService.AllowP2PMessagesFromPeer(mailboxPeer)
	if err != nil {
		writeExitMessageFunc = func() { logger.Error("Failed to allow P2P messages from peer", "error", err) }
		return
	}

	clientRPCClient := clientBackend.StatusNode().RPCClient()

	// TODO: Replace chat implementation with github.com/status-im/status-go-sdk
	_, topic, _, err := joinPublicChat(clientWhisperService, clientRPCClient, *publicChannel)
	if err != nil {
		writeExitMessageFunc = func() { logger.Error("Failed to join public chat", "error", err) }
		return
	}

	// watch for envelopes to be available in filters in the client
	envelopeAvailableWatcher := make(chan whisper.EnvelopeEvent, 1024)
	sub := clientWhisperService.SubscribeEnvelopeEvents(envelopeAvailableWatcher)
	defer sub.Unsubscribe()

	// watch for mailserver responses in the client
	mailServerResponseWatcher := make(chan whisper.EnvelopeEvent, 1024)
	sub = clientWhisperService.SubscribeEnvelopeEvents(mailServerResponseWatcher)
	defer sub.Unsubscribe()

	// request messages from mailbox
	requestID, err := requestHistoricMessages(
		clientRPCClient,
		mailboxPeerStr,
		mailServerKeyID,
		topic.String(),
		clientWhisperService.GetCurrentTime().Add(-time.Duration(*period)*time.Second),
		time.Unix(0, 0),
		1, "")
	if err != nil {
		exitCode = 2
		writeExitMessageFunc = func() { logger.Error("Error requesting historic messages from mailserver", "error", err) }
		return
	}

	// wait for mailserver response
	resp, err := waitForMailServerResponse(mailServerResponseWatcher, requestID, 10*time.Second)
	if err != nil {
		exitCode = 3
		writeExitMessageFunc = func() { logger.Error("Error waiting for mailserver response", "error", err) }
		return
	}

	// wait for last envelope sent by the mailserver to be available for filters
	err = waitForEnvelopeEvents(envelopeAvailableWatcher, []string{resp.LastEnvelopeHash.String()}, whisper.EventEnvelopeAvailable)
	if err != nil {
		exitCode = 4
		writeExitMessageFunc = func() { logger.Error("Error waiting for envelopes to be available to client filter", "error", err) }
		return
	}

	exitCode = 0
}

// makeNodeConfig parses incoming CLI options and returns node configuration object
func makeNodeConfig() (*params.NodeConfig, error) {
	err := error(nil)

	workDir := ""
	if path.IsAbs(*homePath) {
		workDir = *homePath
	} else {
		workDir, err = filepath.Abs(filepath.Dir(os.Args[0]))
		if err == nil {
			workDir = path.Join(workDir, *homePath)
		}
	}
	if err != nil {
		return nil, err
	}

	nodeConfig, err := params.NewNodeConfig(path.Join(workDir, ".ethereum"), "", uint64(params.RopstenNetworkID))
	if err != nil {
		return nil, err
	}

	if *logLevel != "" {
		nodeConfig.LogLevel = *logLevel
		nodeConfig.LogEnabled = true
	}

	if *logFile != "" {
		nodeConfig.LogFile = *logFile
	}

	nodeConfig.NoDiscovery = true

	return whisperConfig(nodeConfig)
}

// whisperConfig creates node configuration object from flags
func whisperConfig(nodeConfig *params.NodeConfig) (*params.NodeConfig, error) {
	whisperConfig := nodeConfig.WhisperConfig
	whisperConfig.Enabled = true
	whisperConfig.LightClient = true
	whisperConfig.MinimumPoW = *minPow
	whisperConfig.TTL = *ttl
	whisperConfig.EnableNTPSync = true

	return nodeConfig, nil
}

func startClientNode() (*api.StatusBackend, func(), error) {
	config, err := makeNodeConfig()
	if err != nil {
		return nil, func() { logger.Error("Error creating node config", "error", err) }, err
	}
	clientBackend := api.NewStatusBackend()
	err = clientBackend.StartNode(config)
	if err != nil {
		return nil, func() { logger.Error("Node start failed", "error", err) }, err
	}
	return clientBackend, func() { _ = clientBackend.StopNode() }, err
}

// requestHistoricMessages asks a mailnode to resend messages.
func requestHistoricMessages(rpcCli *rpc.Client, mailboxEnode, mailServerKeyID, topic string, from, to time.Time, limit int, cursor string) (common.Hash, error) {
	resp := rpcCli.CallRaw(`{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "shhext_requestMessages",
		"params": [{
					"mailServerPeer":"` + mailboxEnode + `",
					"topic":"` + topic + `",
					"symKeyID":"` + mailServerKeyID + `",
					"from":` + strconv.FormatInt(from.Unix(), 10) + `,
					"to":` + strconv.FormatInt(to.Unix(), 10) + `,
					"limit": ` + fmt.Sprintf("%d", limit) + `,
					"cursor": "` + cursor + `"
		}]
	}`)
	reqMessagesResp := baseRPCResponse{}
	err := json.Unmarshal([]byte(resp), &reqMessagesResp)
	logger.Info(resp)
	if err != nil {
		return common.Hash{}, err
	}

	if reqMessagesResp.Error != nil {
		return common.Hash{}, createErrorFromJSONError(reqMessagesResp.Error)
	}

	switch hash := reqMessagesResp.Result.(type) {
	case string:
		if !strings.HasPrefix(hash, "0x") {
			return common.Hash{}, errors.New("missing 0x prefix in shhext_requestMessages RPC call response")
		}
		b, err := hex.DecodeString(hash[2:])
		return common.BytesToHash(b), err
	default:
		err = fmt.Errorf("expected a hash, received %+v", reqMessagesResp.Result)
	}

	return common.Hash{}, err
}

func joinPublicChat(w *whisper.Whisper, rpcClient *rpc.Client, name string) (string, whisper.TopicType, string, error) {
	keyID, err := w.AddSymKeyFromPassword(name)
	if err != nil {
		return "", whisper.TopicType{}, "", err
	}

	h := sha3.NewKeccak256()
	_, err = h.Write([]byte(name))
	if err != nil {
		return "", whisper.TopicType{}, "", err
	}
	fullTopic := h.Sum(nil)
	topic := whisper.BytesToTopic(fullTopic)

	filterID, err := createGroupChatMessageFilter(rpcClient, keyID, topic.String())

	return keyID, topic, filterID, err
}

// createGroupChatMessageFilter create message filter with symmetric encryption.
func createGroupChatMessageFilter(rpcCli *rpc.Client, symkeyID string, topic string) (string, error) {
	resp := rpcCli.CallRaw(`{
			"jsonrpc": "2.0",
			"method": "shh_newMessageFilter", "params": [
				{"type": "sym", "symKeyID": "` + symkeyID + `", "topics": [ "` + topic + `"], "allowP2P":true}
			],
			"id": 1
		}`)

	msgFilterResp := returnedIDResponse{}
	err := json.Unmarshal([]byte(resp), &msgFilterResp)
	messageFilterID := msgFilterResp.Result
	if err == nil && msgFilterResp.Error != nil {
		err = createErrorFromJSONError(msgFilterResp.Error)
	}
	return messageFilterID, err
}

func createErrorFromJSONError(jsonError interface{}) error {
	errorObjMap := jsonError.(map[string]interface{})
	if errorObjMap != nil {
		return fmt.Errorf("error %d: %v", errorObjMap["code"], errorObjMap["message"])
	}
	return nil
}

func waitForMailServerResponse(events chan whisper.EnvelopeEvent, requestID common.Hash, timeout time.Duration) (*whisper.MailServerResponse, error) {
	timeoutTimer := time.NewTimer(timeout)
	for {
		select {
		case event := <-events:
			if event.Hash == requestID {
				resp, err := decodeMailServerResponse(event)
				if resp != nil || err != nil {
					timeoutTimer.Stop()
					return resp, err
				}
			}
		case <-timeoutTimer.C:
			return nil, errors.New("timed out waiting for mailserver response")
		}
	}
}

func decodeMailServerResponse(event whisper.EnvelopeEvent) (response *whisper.MailServerResponse, err error) {
	switch event.Event {
	case whisper.EventMailServerRequestCompleted:
		resp, ok := event.Data.(*whisper.MailServerResponse)
		if !ok {
			err = errors.New("failed to convert event to a *MailServerResponse")
		}

		response = resp
	case whisper.EventMailServerRequestExpired:
		err = errors.New("no messages available from mailserver")
	}

	return
}

func waitForEnvelopeEvents(events chan whisper.EnvelopeEvent, hashes []string, event whisper.EventType) error {
	check := make(map[string]struct{})
	for _, hash := range hashes {
		check[hash] = struct{}{}
	}

	timeout := time.NewTimer(time.Second * 5)
	for {
		select {
		case e := <-events:
			if e.Event == event {
				delete(check, e.Hash.String())
				if len(check) == 0 {
					timeout.Stop()
					return nil
				}
			}
		case <-timeout.C:
			return fmt.Errorf("timed out while waiting for event on envelopes. event: %s", event)
		}
	}
}

type returnedIDResponse struct {
	Result string
	Error  interface{}
}
type baseRPCResponse struct {
	Result interface{}
	Error  interface{}
}
