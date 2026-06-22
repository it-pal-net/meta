package main

import (
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"go.mau.fi/mautrix-meta/pkg/connector"
)

// Information to find out exactly which commit the bridge was built from.
// These are filled at build time with the -X linker flag.
var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var m = mxmain.BridgeMain{
	Name:        "mautrix-meta",
	URL:         "https://github.com/mautrix/meta",
	Description: "A Matrix-Meta puppeting bridge.",
	Version:     "26.06",
	SemCalVer:   true,
	Connector:   &connector.MetaConnector{},
}

func main() {
	m.InitVersion(Tag, Commit, BuildTime)
	m.PostStart = func() {
		// Force-enable batch sending so on-demand history backfill works against
		// our standard Synapse. Synapse does not advertise com.beeper.batch_sending
		// in /versions, so the connector leaves BatchSending=false after Start; we
		// flip it here (PostStart runs after the version fetch). The bridge then
		// routes backfill through
		// POST /_matrix/client/unstable/com.beeper.backfill/.../batch_send, which
		// the synccontact_backfill Synapse module implements (nginx rewrites that
		// path). The backfill queue (br.RunBackfillQueue) is launched during bridge
		// start — before this hook — and already bailed because the capability was
		// false, so re-launch it. Only when we actually flip false->true.
		// Mirrors bridges/whatsapp/cmd/mautrix-whatsapp/main.go; see
		// docs/chat/bridges/whatsapp/history-backfill.md.
		if m.Matrix.Capabilities != nil && !m.Matrix.Capabilities.BatchSending {
			m.Matrix.Capabilities.BatchSending = true
			go m.Bridge.RunBackfillQueue()
		}
	}
	m.Run()
}
