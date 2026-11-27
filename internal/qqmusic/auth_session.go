package qqmusic

import (
	"context"
	"errors"
)

var sessionRefreshGate = make(chan struct{}, 1)

// activeSession renews locally expired credentials before an authenticated operation.
// A musickey's expiry does not imply that its refresh material has expired too.
func activeSession() (Session, error) {
	return activeSessionContextMode(context.Background(), false)
}

// activeSessionForAccount retries renewal even after an earlier refresh failure.
func activeSessionForAccount() (Session, error) {
	return activeSessionContextMode(context.Background(), true)
}

func activeSessionContext(ctx context.Context) (Session, error) {
	return activeSessionContextMode(ctx, false)
}

func activeSessionContextMode(ctx context.Context, force bool) (Session, error) {
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
		return renewSessionContextMode(ctx, s, force)
	}
	return s, nil
}

// renewSession shares a successful renewal with requests waiting on the same key.
func renewSession(prior Session) (Session, error) {
	return renewSessionContextMode(context.Background(), prior, true)
}

func renewSessionContext(ctx context.Context, prior Session) (Session, error) {
	return renewSessionContextMode(ctx, prior, false)
}

func renewSessionContextMode(ctx context.Context, prior Session, force bool) (Session, error) {
	if err := ctx.Err(); err != nil {
		return prior, err
	}
	authState.RLock()
	version := authState.version
	if !force && authState.refreshFailed && authState.session == prior {
		err := authState.refreshErr
		authState.RUnlock()
		return prior, err
	}
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
	if !force && authState.refreshFailed {
		err := authState.refreshErr
		authState.Unlock()
		return current, err
	}
	authState.Unlock()
	s, err := refreshSession(ctx, prior)
	if err != nil {
		// A caller giving up does not mean the credentials failed to renew.
		if cause := ctx.Err(); cause != nil {
			return prior, cause
		}
		if errors.Is(err, context.Canceled) {
			return prior, err
		}
		stage := "refresh"
		var failure *LoginError
		if errors.As(err, &failure) && failure.Step == "device-bootstrap" {
			stage = "device-bootstrap"
		}
		failed := sessionFailure(stage, prior, err)
		authState.Lock()
		if authState.session == prior && authState.version == version {
			authState.refreshFailed = true
			authState.refreshErr = failed
		}
		authState.Unlock()
		return prior, failed
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
	authState.refreshFailed, authState.refreshErr = false, nil
	authState.version++
	return s, saveSessionLocked()
}

var ErrLoginRequired = errors.New("登录续期失败，请重新登录")

type loginPromptError struct {
	cause error
}

func (e *loginPromptError) Error() string        { return ErrLoginRequired.Error() }
func (e *loginPromptError) Unwrap() error        { return e.cause }
func (e *loginPromptError) Is(target error) bool { return target == ErrLoginRequired }

func sessionRefreshError() error {
	authState.RLock()
	failed, err := authState.refreshFailed, authState.refreshErr
	authState.RUnlock()
	if !failed {
		return nil
	}
	return &loginPromptError{cause: err}
}

func isCredentialRejection(err error) bool {
	var rejected *ProtocolError
	return errors.As(err, &rejected) && rejected.GlobalCode == 0 && (rejected.Code == 1000 || rejected.Code == 104400 || rejected.Code == 104401)
}
