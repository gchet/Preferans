package game

import (
	"fmt"
	"slices"
	"testing"
)

func TestThroughHerePositionsAndSixSpades(t *testing.T) {
	for _, n := range []int{3, 4} {
		for dealer := 0; dealer < n; dealer++ {
			for seat := 0; seat < n; seat++ {
				for bidder := 0; bidder < n; bidder++ {
					if seat == bidder || (n == 4 && (seat == dealer || bidder == dealer)) {
						continue
					}
					for _, bid := range []Contract{{Level: 6, Suit: 0}, {Level: 6, Suit: 1}, {Level: 7, Suit: 0}} {
						s, _ := New(n, 30)
						s.Dealer = dealer
						s.Stage = "auction"
						s.Turn = seat
						s.Declarer = bidder
						s.Bid = &bid
						s.Passed = make([]bool, n)
						s.Spoken = make([]bool, n)
						for _, p := range s.Active() {
							s.Passed[p] = p != seat && p != bidder
						}
						want := (bidder+1)%n != seat && !(bid.Level == 6 && bid.Suit == 0)
						if got := slices.Contains(s.View(seat).Contracts, bid); got != want {
							t.Fatalf("n=%d dealer=%d seat=%d bidder=%d bid=%v got=%v", n, dealer, seat, bidder, bid, got)
						}
						next, err := Apply(s, Command{ID: ID(), Seat: seat, Revision: s.Revision, Action: "bid", Contract: &bid}, nil)
						if (err == nil) != want {
							t.Fatalf("unexpected acceptance: %v", err)
						}
						if want {
							next = next.Clone()
							if next.HereOwner == nil || *next.HereOwner != seat {
								t.Fatal("first caller not persisted")
							}
							if slices.Contains(next.View(bidder).Contracts, bid) {
								t.Fatal("opponent can answer here with here")
							}
							if _, err := Apply(next, Command{ID: ID(), Seat: bidder, Revision: next.Revision, Action: "bid", Contract: &bid}, nil); err == nil {
								t.Fatal("opponent's here accepted")
							}
						}
					}
				}
			}
		}
	}
}

func TestThroughHereOwnerSurvivesRaiseAndResetsOnDeal(t *testing.T) {
	s, _ := New(4, 30)
	s.Dealer = 3
	s.Stage = "auction"
	s.Turn = 2
	s.Declarer = 0
	s.Passed = []bool{false, true, false, false}
	s.Spoken = make([]bool, 4)
	s.Bid = &Contract{Level: 7, Suit: 0}
	s = step(t, s, 2, "bid", s.Bid)
	higher := Contract{Level: 8, Suit: 0}
	s = step(t, s, 0, "bid", &higher)
	if !s.canRepeatBid(2) {
		t.Fatal("first caller lost priority after raise")
	}
	s = step(t, s, 2, "bid", &higher)
	if s.canRepeatBid(0) {
		t.Fatal("opponent acquired priority after raise")
	}
	if err := s.deal(nil); err != nil {
		t.Fatal(err)
	}
	if s.HereOwner != nil {
		t.Fatal("priority leaked into next deal")
	}
}

func TestHereRequiresTwoBiddersAndSeniorHand(t *testing.T) {
	for _, n := range []int{3, 4} {
		for dealer := 0; dealer < n; dealer++ {
			for senior := 0; senior < 2; senior++ {
				for junior := senior + 1; junior < 3; junior++ {
					for _, two := range []bool{false, true} {
						for _, respondingSenior := range []bool{false, true} {
							t.Run(fmt.Sprintf("%d/dealer%d/hands%d-%d/two%v/senior%v", n, dealer, senior, junior, two, respondingSenior), func(t *testing.T) {
								s, _ := New(n, 30)
								rules := DefaultRules()
								rules.HereConvention = "seniority"
								s.Rules = &rules
								s.Dealer = dealer
								s.Stage = "auction"
								s.Passed = make([]bool, n)
								s.Spoken = make([]bool, n)
								active := s.Active()
								seat, bidder := active[senior], active[junior]
								if !respondingSenior {
									seat, bidder = bidder, seat
								}
								if two {
									for _, p := range active {
										if p != seat && p != bidder {
											s.Passed[p] = true
										}
									}
								}
								s.Turn = seat
								s.Declarer = bidder
								s.Bid = &Contract{Level: 7, Suit: 2}
								s.Spoken[seat] = true // ordinary bids may be repeated after speaking
								want := two && respondingSenior
								v := s.View(seat)
								visible := false
								for _, b := range v.Contracts {
									if b == *s.Bid {
										visible = true
									}
								}
								if visible != want {
									t.Fatalf("repeat offered=%v, want %v", visible, want)
								}
								next, err := Apply(s, Command{ID: ID(), Seat: seat, Revision: s.Revision, Action: "bid", Contract: s.Bid}, nil)
								if (err == nil) != want {
									t.Fatalf("repeat accepted=%v, want %v: %v", err == nil, want, err)
								}
								if want {
									if next.Declarer != seat || *next.Bid != *s.Bid || next.Turn != bidder {
										t.Fatal("here did not transfer bid and turn")
									}
									if _, err := Apply(next, Command{ID: ID(), Seat: bidder, Revision: next.Revision, Action: "bid", Contract: next.Bid}, nil); err == nil {
										t.Fatal("junior answered here with here")
									}
								}
							})
						}
					}
				}
			}
		}
	}
}
