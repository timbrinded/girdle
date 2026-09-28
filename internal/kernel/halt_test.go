package kernel

import "testing"

func TestTheMoreSeriousHaltWins(t *testing.T) {
	nudge := halt{kind: haltNudge, text: "retry", reason: "crosscheck"}
	user := halt{kind: haltUser, reason: "tripwire: rm -rf /"}
	for _, order := range [][]halt{{nudge, user}, {user, nudge}} {
		var s Session
		for _, h := range order {
			s.stopTurn(h)
		}
		if s.halt != user {
			t.Errorf("after %v: halt = %v, want %v", order, s.halt, user)
		}
	}
}
