package app

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
	"lilt/internal/qqmusic"
)

// Opening the account's page refreshes membership, and a failed result can be retried.
func TestOwnMembershipRefreshAndRetry(t *testing.T) {
	fakeSubscriptions(t)
	old := userMembership
	t.Cleanup(func() { userMembership = old })
	asked := make(chan struct{}, 4)
	userMembership = func() (qqmusic.Membership, error) {
		asked <- struct{}{}
		return qqmusic.MembershipGreen, nil
	}
	a := ownPageApp()
	a.open(route{page: pageUser, id: 9})
	wait(t, asked, "membership status")
	d := a.users[9]
	a.acceptMembership(9, d, qqmusic.MembershipGreen, nil)
	if d.membership != qqmusic.MembershipGreen || d.memberLoading || d.memberErr {
		t.Fatalf("membership result not applied: %+v", d)
	}
	a.acceptMembership(9, d, qqmusic.MembershipUnknown, errors.New("offline"))
	tt := ui.NewTester(a.view, 1000, 800)
	tt.Frame()
	if err := tt.Click("重试会员状态"); err != nil {
		t.Fatal(err)
	}
	wait(t, asked, "membership retry")
	if !d.memberLoading || d.memberErr {
		t.Fatal("retry did not restart membership query")
	}
}

// A result from the previous sign-in cannot replace the current account's membership.
func TestMembershipRejectsOldAccount(t *testing.T) {
	a := ownPageApp()
	old := a.users[9]
	oldMembership := old.membership
	fresh := &userData{membership: qqmusic.MembershipFree}
	a.users[9] = fresh
	a.acceptMembership(9, old, qqmusic.MembershipSuper, nil)
	if fresh.membership != qqmusic.MembershipFree || old.membership != oldMembership {
		t.Fatal("accepted a result for the replaced account page")
	}
	a.profile = nil
	a.acceptMembership(9, fresh, qqmusic.MembershipSuper, nil)
	if fresh.membership != qqmusic.MembershipFree {
		t.Fatal("accepted a signed-out membership result")
	}
}

// Expiry failures keep the membership tier and can be retried without accepting an old account's result.
func TestMembershipExpiryRetryAndAccountChange(t *testing.T) {
	fakeSubscriptions(t)
	asked := make(chan struct{}, 1)
	userMembershipExpiry = func(level qqmusic.Membership) (string, error) {
		asked <- struct{}{}
		return "2027-06-30", nil
	}
	a := ownPageApp()
	d := a.users[9]
	a.acceptMembershipExpiry(9, d, qqmusic.MembershipGreen, "", errors.New("offline"))
	if !d.expiryErr || d.membership != qqmusic.MembershipGreen {
		t.Fatal("expiry failure changed the membership tier or was not recorded")
	}
	a.route = route{page: pageUser, id: 9}
	tt := ui.NewTester(a.view, 1000, 800)
	tt.Frame()
	if err := tt.Click("重试到期时间"); err != nil {
		t.Fatal(err)
	}
	wait(t, asked, "membership expiry retry")
	if !d.expiryLoading || d.expiryErr {
		t.Fatal("expiry retry did not start")
	}
	a.users[9] = &userData{membership: qqmusic.MembershipFree}
	a.acceptMembershipExpiry(9, d, qqmusic.MembershipGreen, "2028-01-01", nil)
	if d.memberExpiry != "2027-06-30" {
		t.Fatal("accepted expiry from the previous account")
	}
}

// A membership-status failure still requests expiry dates from the separate client endpoint.
func TestMembershipStatusFailureStillRequestsDates(t *testing.T) {
	fakeSubscriptions(t)
	asked := make(chan struct{}, 1)
	userMembershipDates = func() (string, string, error) {
		asked <- struct{}{}
		return "2027-06-30", "2028-01-01", nil
	}
	a := ownPageApp()
	d := a.users[9]
	a.acceptMembership(9, d, qqmusic.MembershipUnknown, errors.New("status unavailable"))
	wait(t, asked, "independent membership dates")
	if !d.memberErr || !d.expiryLoading {
		t.Fatal("status failure did not start the independent date request")
	}
}
