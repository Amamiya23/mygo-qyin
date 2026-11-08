package qqmusic

import (
	"context"
	"encoding/json"
	"errors"
	"log"
)

var sessionRefreshGate = make(chan struct{}, 1)

// activeSession renews locally expired credentials before an authenticated operation.
// A musickey's expiry does not imply that its refresh material has expired too.
func activeSession() (Session, error) {
	return activeSessionContext(context.Background())
}

func activeSessionContext(ctx context.Context) (Session, error) {
	s := sessionSnapshot()
	if !s.valid() && s.canRefresh() {
		return renewSessionContext(ctx, s)
	}
	return s, nil
}

// renewSession shares a successful renewal with requests waiting on the same key.
func renewSession(prior Session) (Session, error) {
	return renewSessionContext(context.Background(), prior)
}

func renewSessionContext(ctx context.Context, prior Session) (Session, error) {
	if err := ctx.Err(); err != nil {
		return prior, err
	}
	select {
	case sessionRefreshGate <- struct{}{}:
		defer func() { <-sessionRefreshGate }()
	case <-ctx.Done():
		return prior, ctx.Err()
	}
	if current := sessionSnapshot(); current != prior {
		return current, nil
	}
	s, err := refreshSession(ctx, prior)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return prior, err
		}
		// Only diagnostic types and public status codes are logged, never credentials.
		var response *loginResponseError
		if errors.As(err, &response) {
			log.Printf("qqmusic: refresh loginType=%d tmeLoginType=%d kind=%s gateway=%d code=%d responseLoginType=%d", prior.LoginType, prior.LoginType, response.Kind, response.Gateway, response.Business, response.LoginType)
		} else {
			log.Printf("qqmusic: refresh loginType=%d tmeLoginType=%d: %s", prior.LoginType, prior.LoginType, LoginErrorMessage(err))
		}
		return prior, err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return prior, err
	}
	authState.Lock()
	defer authState.Unlock()
	// A completed refresh must not sign back in after logout or an account change.
	if authState.session != prior {
		return authState.session, nil
	}
	if err = writePersistedSession(b); err != nil {
		return prior, err
	}
	authState.session = s
	return s, nil
}

func isCredentialRejection(err error) bool {
	var rejected *ProtocolError
	return errors.As(err, &rejected) && rejected.GlobalCode == 0 && (rejected.Code == 1000 || rejected.Code == 104400 || rejected.Code == 104401)
}
