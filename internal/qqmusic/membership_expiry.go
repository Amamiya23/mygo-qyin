package qqmusic

import (
	"encoding/json"
	"errors"
	"time"
)

// UserMembershipExpiry returns a paid membership's expiry date, or an empty
// string when the service does not provide a reliable date for that tier.
func UserMembershipExpiry(level Membership) (string, error) {
	if level != MembershipGreen && level != MembershipSuper {
		return "", nil
	}
	green, super, err := UserMembershipDates()
	if err != nil {
		return "", err
	}
	if level == MembershipSuper {
		return super, nil
	}
	return green, nil
}

// UserMembershipDates reads each explicitly identified membership expiry independently.
func UserMembershipDates() (green, super string, err error) {
	s := sessionSnapshot()
	if s.MusicID == "" || s.MusicKey == "" {
		return "", "", errors.New("请先登录 QQ 音乐")
	}
	var data struct {
		Identity map[string]json.RawMessage `json:"identity"`
	}
	if err := rpc("VipLogin.VipLoginInter", "vip_login_base", map[string]any{}, &data); err != nil {
		return "", "", err
	}
	return membershipDate(data.Identity["overdate"]), membershipDate(data.Identity["HugeVipEnd"]), nil
}

func membershipDate(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05"} {
			if date, err := time.Parse(layout, value); err == nil && date.Year() > 1970 {
				return date.Format("2006-01-02")
			}
		}
		return ""
	}
	var seconds int64
	if json.Unmarshal(raw, &seconds) != nil || seconds <= 0 {
		return ""
	}
	date := time.Unix(seconds, 0).In(time.FixedZone("CST", 8*60*60))
	if date.Year() < 1971 || date.Year() > 9999 {
		return ""
	}
	return date.Format("2006-01-02")
}
