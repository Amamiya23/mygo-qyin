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
