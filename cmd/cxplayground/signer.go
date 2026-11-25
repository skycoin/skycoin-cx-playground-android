package main

import (
	"fmt"
	"io"

	"github.com/0magnet/gio/layout"
	"github.com/0magnet/gio/widget"
	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// signerQuestion asks the user whether a program may run whose signature
// is valid but whose signer is not trusted (D27).
type signerQuestion struct {
	key    cipher.PubKey
	answer chan bool
}

// confirmSigner shows the signer question and waits for the answer. It
// runs on the goroutine that prepares the program, never on the UI one.
func (u *ui) confirmSigner(key cipher.PubKey) bool {
	q := &signerQuestion{key: key, answer: make(chan bool, 1)}
	u.askMu.Lock()
	u.ask = q
	u.askMu.Unlock()
	u.trace.printf("signer ask key=%s", key.Hex())
	u.w.Invalidate()
	run := <-q.answer
	u.trace.printf("signer answer run=%t", run)
	return run
}

// answerSigner answers the open signer question, if there is one.
func (u *ui) answerSigner(run bool) {
	u.askMu.Lock()
	defer u.askMu.Unlock()
	if u.ask != nil {
		u.ask.answer <- run
		u.ask = nil
	}
}

// layoutSignerQuestion shows the open signer question with its two
// answers, and nothing when no question is open.
func (u *ui) layoutSignerQuestion(gtx layout.Context) layout.Dimensions {
	u.askMu.Lock()
	q := u.ask
	u.askMu.Unlock()
	if q == nil {
		return layout.Dimensions{}
	}
	if u.askRun.Clicked(gtx) {
		u.answerSigner(true)
		u.w.Invalidate()
	}
	if u.askRefuse.Clicked(gtx) {
		u.answerSigner(false)
		u.w.Invalidate()
	}
	button := func(c *widget.Clickable, label string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.text(gtx, link, label, false)
				})
			})
		})
	}
	return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.text(gtx, refused, "the signature is valid, but the signer is not trusted:", true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.text(gtx, fg, q.key.Hex(), true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx, button(&u.askRun, "[ run it ]  "), button(&u.askRefuse, "[ refuse ]"))
			}),
		)
	})
}

// reportSigner writes the signer of a verified program and whether it is
// trusted, then asks confirm when it is not. It returns an error when the
// program must not run.
func reportSigner(out io.Writer, signer cipher.PubKey, trusted []cipher.PubKey, confirm func(cipher.PubKey) bool) error {
	if programblocks.KeyListed(trusted, signer) {
		fmt.Fprintf(out, "signer %s verified, trusted\n", signer.Hex())
		return nil
	}
	fmt.Fprintf(out, "signer %s verified, not trusted, asking the user\n", signer.Hex())
	return programblocks.ConfirmSigner(signer, trusted, confirm)
}
