package signal

const (
  X3dhBundleCreated = "x3dh.bundle.created"
  X3dhBundleCreateFailed = "x3dh.bundle.create.failed"
)

func SendX3dhBundleCreated(bundle string) {
  send(X3dhBundleCreated, bundle)
}

func SendX3dhBundleCreateFailed(reason string) {
  send(X3dhBundleCreateFailed, reason)
}
