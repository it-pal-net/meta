package connector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/id"
)

// SyncContact fork extension: a coarse login sync-status used to drive the
// "initial sync in progress" indicator on the unlinked-chat list, mirroring the
// equivalent endpoint in bridges/whatsapp. Unlike WhatsApp, mautrix-meta has no
// per-login pending-message/portal bookkeeping, so this reports a coarse phase
// derived from the bridge connection state plus whether the initial inbox table
// (the thread list portals are created from) has been handled.
const (
	MetaSyncPhaseConnecting = "connecting"
	// Reuses the WhatsApp phase id so the chat API / frontend phase mapping is
	// provider-neutral: "still creating the inbox portals".
	MetaSyncPhaseSyncing = "creating_portals"
	MetaSyncPhaseReady   = "ready"
)

type LoginSessionStatus struct {
	BridgeState string `json:"bridge_state"`
	Connected   bool   `json:"connected"`
}

type LoginSyncStatusResponse struct {
	LoginID             string             `json:"login_id"`
	Session             LoginSessionStatus `json:"session"`
	Phase               string             `json:"phase"`
	InitialSyncComplete bool               `json:"initial_sync_complete"`
	ReadyForDiscovery   bool               `json:"ready_for_discovery"`
}

func metaSyncPhase(connected, initialSyncComplete bool) string {
	if !connected {
		return MetaSyncPhaseConnecting
	}
	if !initialSyncComplete {
		return MetaSyncPhaseSyncing
	}
	return MetaSyncPhaseReady
}

func bridgeStateEventName(userLogin *bridgev2.UserLogin) string {
	if userLogin == nil || userLogin.BridgeState == nil {
		return ""
	}
	return string(userLogin.BridgeState.GetPrev().StateEvent)
}

// GetLoginSyncStatusForLoginID builds the coarse sync status for one Meta login.
// Returns (nil, nil) when the login does not exist or is not owned by ownerMXID,
// so the caller can answer 404 without leaking other users' logins.
func GetLoginSyncStatusForLoginID(
	ctx context.Context,
	bridge *bridgev2.Bridge,
	ownerMXID id.UserID,
	loginID networkid.UserLoginID,
) (*LoginSyncStatusResponse, error) {
	userLogin, err := bridge.GetExistingUserLoginByID(ctx, loginID)
	if err != nil {
		return nil, err
	} else if userLogin == nil || userLogin.UserMXID != ownerMXID {
		return nil, nil
	}

	initialSyncComplete := false
	if metaClient, ok := userLogin.Client.(*MetaClient); ok && metaClient != nil {
		initialSyncComplete = metaClient.initialTableHandled.Load()
	}

	bridgeState := bridgeStateEventName(userLogin)
	connected := bridgeState == string(status.StateConnected)
	phase := metaSyncPhase(connected, initialSyncComplete)

	return &LoginSyncStatusResponse{
		LoginID: string(loginID),
		Session: LoginSessionStatus{
			BridgeState: bridgeState,
			Connected:   connected,
		},
		Phase:               phase,
		InitialSyncComplete: initialSyncComplete,
		ReadyForDiscovery:   connected && initialSyncComplete,
	}, nil
}
