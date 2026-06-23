package main

// This file holds SyncContact fork-specific provisioning endpoints served under
// the bridgev2 provisioning API (/_matrix/provision/v3). These handlers are new
// extensions that the SyncContact chat API drives directly. The logic is
// provider-neutral bridgev2 framework code; it mirrors the equivalent endpoint
// in bridges/whatsapp/cmd/mautrix-whatsapp/customprovision.go.

import (
	"net/http"

	"github.com/rs/zerolog/hlog"
	"go.mau.fi/util/exhttp"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

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
