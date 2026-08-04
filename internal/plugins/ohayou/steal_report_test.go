package ohayou

import (
	"testing"
	"time"
)

func TestOfferIsTakenOnce(t *testing.T) {
	o := newOffers()

	ch, done, ok := o.open("alice")
	if !ok {
		t.Fatal("could not open an offer")
	}
	defer done()

	if !o.take("alice") {
		t.Error("take() found no offer")
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Error("taking the offer did not wake the waiter")
	}

	if o.take("alice") {
		t.Error("the same offer was taken twice")
	}
}

// One victim reporting must not answer for another.
func TestOffersArePerUser(t *testing.T) {
	o := newOffers()
	aliceCh, aliceDone, _ := o.open("alice")
	defer aliceDone()
	_, bobDone, _ := o.open("bob")
	defer bobDone()

	if !o.take("bob") {
		t.Fatal("bob's offer was not there")
	}
	select {
	case <-aliceCh:
		t.Error("bob reporting closed alice's offer")
	default:
	}
}

func TestASecondOfferToTheSameUserIsRefused(t *testing.T) {
	o := newOffers()
	_, done, ok := o.open("alice")
	if !ok {
		t.Fatal("the first offer was refused")
	}
	defer done()

	if _, _, ok := o.open("alice"); ok {
		t.Error("a second offer to the same user was opened")
	}
}

func TestClosingAnOfferStopsTheWait(t *testing.T) {
	o := newOffers()
	_, done, _ := o.open("alice")
	done()

	if o.take("alice") {
		t.Error("an offer that was closed could still be taken")
	}
}
