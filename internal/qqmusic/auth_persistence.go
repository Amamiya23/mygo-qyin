package qqmusic

import (
	"errors"
	"fmt"
	"log"
	"syscall"
)

var errSessionTooLarge = errors.New("qqmusic: session exceeds credential store limit")

// SessionError identifies the recovery stage without printing its underlying cause.
type SessionError struct {
	Stage string
	Err   error
}

func (e *SessionError) Error() string { return SessionErrorMessage(e) }
func (e *SessionError) Unwrap() error { return e.Err }

func sessionFailure(stage string, s Session, err error) error {
	var status loginHTTPError
	var system syscall.Errno
	var response *loginResponseError
	var protocol *ProtocolError
	gateway, code := 0, 0
	if errors.As(err, &response) {
		gateway, code = response.Gateway, response.Business
	} else if errors.As(err, &protocol) {
		gateway, code = protocol.GlobalCode, protocol.Code
	}
	errors.As(err, &status)
	errors.As(err, &system)
	log.Printf("qqmusic: session stage=%s http=%d system=%d gateway=%d code=%d refreshKey=%t refreshToken=%t",
		stage, int(status), uint64(system), gateway, code, s.RefreshKey != "", s.RefreshToken != "")
	return &SessionError{Stage: stage, Err: err}
}

// SessionNeedsLogin is deliberately limited to locally unusable saved data.
// Provider business errors alone do not prove that refresh material is revoked.
func SessionNeedsLogin(err error) bool {
	var failure *SessionError
	return errors.As(err, &failure) && (failure.Stage == "decode" || failure.Stage == "unusable")
}

// SessionNeedsReload marks failures that occurred before credentials reached memory.
func SessionNeedsReload(err error) bool {
	var failure *SessionError
	return errors.As(err, &failure) && failure.Stage == "read"
}

// SessionErrorMessage never includes underlying storage errors or credential values.
func SessionErrorMessage(err error) string {
	var failure *SessionError
	if errors.As(err, &failure) {
		switch failure.Stage {
		case "read":
			return "无法读取系统保存的登录凭据，请重试恢复（S101）"
		case "decode":
			return "保存的登录凭据已损坏，请重新扫码（S102）"
		case "unusable":
			return "登录凭据已失效且无法续期，请重新扫码（S103）"
		case "save":
			if errors.Is(err, errSessionTooLarge) {
				return "登录凭据超过系统存储大小限制（S105）"
			}
			return "登录凭据保存失败，请重试保存（S104）"
		}
	}
	var response *loginResponseError
	if errors.As(err, &response) {
		if response.Kind == "rejected" {
			return fmt.Sprintf("登录续期失败，请稍后重试（服务码 %d/%d）", response.Gateway, response.Business)
		}
		return "登录续期响应异常，请稍后重试（S107）"
	}
	var protocol *ProtocolError
	if errors.As(err, &protocol) {
		return fmt.Sprintf("登录验证失败，请稍后重试（服务码 %d/%d）", protocol.GlobalCode, protocol.Code)
	}
	return "登录恢复暂时失败，请检查网络后重试（S106）"
}
