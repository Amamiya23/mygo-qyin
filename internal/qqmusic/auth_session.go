package qqmusic

import (
	"context"
	"errors"
)

var sessionRefreshGate = make(chan struct{}, 1)

// activeSession renews locally expired credentials before an authenticated operation.
// A musickey's expiry does not imply that its refresh material has expired too.
func activeSession() (Session, error) {
	return activeSessionContext(context.Background())
}

func activeSessionContext(ctx context.Context) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	authState.Lock()
	if authState.pending {
		err := saveSessionLocked()
		s := authState.session
		authState.Unlock()
		if err != nil {
			return s, err
		}
	} else {
		authState.Unlock()
	}
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
	authState.RLock()
	version := authState.version
	authState.RUnlock()
	select {
	case sessionRefreshGate <- struct{}{}:
		defer func() { <-sessionRefreshGate }()
	case <-ctx.Done():
		return prior, ctx.Err()
	}
	authState.Lock()
	current, changed := authState.session, authState.version != version
	if current != prior || changed {
		var err error
		if authState.pending {
			err = saveSessionLocked()
		}
		authState.Unlock()
		return current, err
	}
	authState.Unlock()
	s, err := refreshSession(ctx, prior)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return prior, err
		}
		stage := "refresh"
		var failure *LoginError
		if errors.As(err, &failure) && failure.Step == "device-bootstrap" {
			stage = "device-bootstrap"
		}
		return prior, sessionFailure(stage, prior, err)
	}
	authState.Lock()
	defer authState.Unlock()
	// A completed refresh must not sign back in after logout or an account change.
	if authState.session != prior || authState.version != version {
		return authState.session, nil
	}
	// A rotated token may already have invalidated the old one. Keep it in
	// memory even if persistence fails, and retry saving before another request.
	authState.session, authState.pending = s, true
	authState.version++
	return s, saveSessionLocked()
}

func isCredentialRejection(err error) bool {
	var rejected *ProtocolError
	return errors.As(err, &rejected) && rejected.GlobalCode == 0 && (rejected.Code == 1000 || rejected.Code == 104400 || rejected.Code == 104401)
}
