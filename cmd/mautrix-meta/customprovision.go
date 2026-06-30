package main

// This file holds SyncContact fork-specific provisioning endpoints served under
// the bridgev2 provisioning API (/_matrix/provision/v3). These handlers are new
// extensions that the SyncContact chat API drives directly. The logic is
// provider-neutral bridgev2 framework code; it mirrors the equivalent endpoint
// in bridges/whatsapp/cmd/mautrix-whatsapp/customprovision.go.

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/hlog"
	"go.mau.fi/util/exhttp"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/connector"
)

// provLoginSyncStatus returns a coarse "initial sync" status for one Meta login,
// used to drive the unlinked-chat sync indicator. Mirrors the WhatsApp bridge's
// endpoint of the same path; see pkg/connector/syncstatus.go.
func provLoginSyncStatus(w http.ResponseWriter, r *http.Request) {
	user := m.Matrix.Provisioning.GetUser(r)
	if user == nil {
		mautrix.MForbidden.WithMessage("Authenticated user not found").Write(w)
		return
	}

	loginID := networkid.UserLoginID(r.PathValue("login_id"))
	if loginID == "" {
		mautrix.MInvalidParam.WithMessage("login_id is required").Write(w)
		return
	}

	status, err := connector.GetLoginSyncStatusForLoginID(
		r.Context(),
		m.Bridge,
		user.MXID,
		loginID,
	)
	if err != nil {
		hlog.FromRequest(r).Err(err).Str("login_id", string(loginID)).Msg("Failed to build login sync status")
		matrix.RespondWithError(w, err, "Internal error loading sync status")
		return
	} else if status == nil {
		mautrix.MNotFound.WithMessage("Login not found").Write(w)
		return
	}

	exhttp.WriteJSONResponse(w, http.StatusOK, status)
}

// deleteLoginPortals deletes all Matrix portal rooms owned by a given login.
// It is used when removing a Meta connection and its associated chat rooms.
func deleteLoginPortals(w http.ResponseWriter, r *http.Request) {
	user := m.Matrix.Provisioning.GetUser(r)
	if user == nil {
		mautrix.MForbidden.WithMessage("Authenticated user not found").Write(w)
		return
	}

	loginID := networkid.UserLoginID(r.PathValue("login_id"))
	if loginID == "" {
		mautrix.MInvalidParam.WithMessage("login_id is required").Write(w)
		return
	}

	portals, err := m.Bridge.GetAllPortalsWithMXID(r.Context())
	if err != nil {
		hlog.FromRequest(r).Err(err).Str("login_id", string(loginID)).Msg("Failed to load portals for login")
		matrix.RespondWithError(w, err, "Internal error loading portals")
		return
	}

	deleted := 0
	for _, portal := range portals {
		if portal.Receiver != loginID {
			continue
		}

		err = portal.Delete(r.Context())
		if err != nil {
			hlog.FromRequest(r).Err(err).
				Str("login_id", string(loginID)).
				Stringer("portal_mxid", portal.MXID).
				Msg("Failed to delete portal from database")
			continue
		}

		err = m.Bridge.Bot.DeleteRoom(r.Context(), portal.MXID, false)
		if err != nil {
			hlog.FromRequest(r).Err(err).
				Str("login_id", string(loginID)).
				Stringer("portal_mxid", portal.MXID).
				Msg("Failed to clean up portal Matrix room")
			continue
		}

		deleted++
	}

	exhttp.WriteJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted})
}

type RelayRequest struct {
	RelayLoginID networkid.UserLoginID `json:"relay_login_id"`
}

// setPortalRelay turns the given (workspace-owned) login into the relaybot for a
// portal, so messages from any authenticated Matrix user in the room are sent to
// Meta over that one login's connection. This is what enables SyncContact's
// backend-owned send model; without it the portal is receive-only. It calls the
// framework's portal.SetRelay directly, bypassing the user-facing !set-relay
// command (and its matrix.relay.enabled gate). Mirrors the WhatsApp and Meta
// Business bridges; see bridges/whatsapp/cmd/mautrix-whatsapp/customprovision.go
// and docs/chat/bridges/relay-mode.md.
func setPortalRelay(w http.ResponseWriter, r *http.Request) {
	roomID := id.RoomID(r.PathValue("roomID"))
	if roomID == "" {
		mautrix.MInvalidParam.WithMessage("Missing room ID").Write(w)
		return
	}

	user := m.Matrix.Provisioning.GetUser(r)
	if user == nil {
		mautrix.MForbidden.WithMessage("Authenticated user not found").Write(w)
		return
	}

	var req RelayRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		mautrix.MBadJSON.WithMessage("Invalid JSON body").Write(w)
		return
	} else if req.RelayLoginID == "" {
		mautrix.MInvalidParam.WithMessage("relay_login_id is required").Write(w)
		return
	}

	relay, err := m.Bridge.GetExistingUserLoginByID(r.Context(), req.RelayLoginID)
	if err != nil {
		hlog.FromRequest(r).Err(err).Str("relay_login_id", string(req.RelayLoginID)).Msg("Failed to load relay login")
		matrix.RespondWithError(w, err, "Internal error loading relay login")
		return
	} else if relay == nil {
		mautrix.MNotFound.WithMessage("Relay login not found").Write(w)
		return
	} else if relay.UserMXID != user.MXID {
		mautrix.MForbidden.WithMessage("Relay login is owned by another user").Write(w)
		return
	} else if !relay.Client.IsLoggedIn() {
		mautrix.MForbidden.WithMessage("Relay login is not connected").Write(w)
		return
	}

	portal, err := m.Bridge.GetPortalByMXID(r.Context(), roomID)
	if err != nil {
		hlog.FromRequest(r).Err(err).Str("room_id", string(roomID)).Msg("Failed to load portal by Matrix room ID")
		matrix.RespondWithError(w, err, "Internal error loading portal")
		return
	} else if portal == nil || portal.MXID == "" {
		mautrix.MNotFound.WithMessage("Portal not found").Write(w)
		return
	}

	err = portal.SetRelay(r.Context(), relay)
	if err != nil {
		hlog.FromRequest(r).Err(err).
			Str("room_id", string(roomID)).
			Str("relay_login_id", string(req.RelayLoginID)).
			Msg("Failed to set portal relay")
		matrix.RespondWithError(w, err, "Internal error setting portal relay")
		return
	}

	exhttp.WriteJSONResponse(w, http.StatusOK, map[string]bool{"ok": true})
}
