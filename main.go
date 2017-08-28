// sshsweeper project main.go
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	mrand "math/rand"
	"net"
	"strconv"

	"golang.org/x/crypto/ssh"

	"github.com/andyleap/SSHTerm"
	tb "github.com/andyleap/SSHTerm/SSHTermbox"

	"github.com/andyleap/imterm"
)

func main() {
	config := &ssh.ServerConfig{
		//Define a function to run when a client attempts a password login
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}

	// You can generate a keypair with 'ssh-keygen -t rsa'
	privateBytes, err := ioutil.ReadFile("id_rsa")
	if err != nil {
		pk, _ := rsa.GenerateKey(rand.Reader, 2048)
		pkBytes := x509.MarshalPKCS1PrivateKey(pk)
		b := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: pkBytes,
		}
		privateBytes = pem.EncodeToMemory(b)

		ioutil.WriteFile("id_rsa", privateBytes, 0777)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatal("Failed to parse private key")
	}

	config.AddHostKey(private)

	// Once a ServerConfig has been configured, connections can be accepted.
	listener, err := net.Listen("tcp", "0.0.0.0:2200")
	if err != nil {
		log.Fatalf("Failed to listen on 2200 (%s)", err)
	}

	st := sshterm.New(config)

	st.Handler = NewGameBoard

	st.Listen(listener)
}

type TermAdapter struct {
	Term *tb.Termbox
}

func (ta *TermAdapter) SetCell(x, y int, ch rune, fg, bg imterm.Attribute) {
	ta.Term.SetCell(x, y, ch, tb.Attribute(fg), tb.Attribute(bg))
}
func (ta *TermAdapter) Size() (w, h int) {
	return ta.Term.Size()
}
func (ta *TermAdapter) Flip() {
	ta.Term.Flush()
}
func (ta *TermAdapter) Clear(bg imterm.Attribute) {
	ta.Term.Clear(tb.ColorDefault, tb.Attribute(bg))
}

type GameBoard struct {
	Term *tb.Termbox
	B    *Board
	it   *imterm.Imterm

	neww, newh int

	refresh chan struct{}

	gamewidth  string
	gameheight string
	gamemines  string
}

func NewGameBoard(term *tb.Termbox) sshterm.Term {
	term.SetInputMode(tb.InputEsc | tb.InputMouse)

	it, _ := imterm.New(&TermAdapter{term})
	gb := &GameBoard{
		Term:    term,
		refresh: make(chan struct{}, 2),
		it:      it,
		neww:    80,
		newh:    40,

		gamewidth:  "20",
		gameheight: "20",
		gamemines:  "30",
	}

	go gb.Run()

	return gb
}

func (gb *GameBoard) Resize(w, h int) {
	if gb.neww != w || gb.newh != h {
		gb.neww, gb.newh = w, h
		gb.Refresh()
	}
}

func (gb *GameBoard) Run() {
	go func() {
		for {
			e := gb.Term.PollEvent()
			switch e.Type {
			case tb.EventMouse:
				button := imterm.MouseNone
				switch e.Key {
				case tb.MouseRelease:
					button = imterm.MouseRelease
				case tb.MouseLeft:
					button = imterm.MouseLeft
				case tb.MouseRight:
					button = imterm.MouseRight
				case tb.MouseMiddle:
					button = imterm.MouseMiddle
				case tb.MouseWheelUp:
					button = imterm.MouseWheelUp
				case tb.MouseWheelDown:
					button = imterm.MouseWheelDown
				}
				gb.it.Mouse(e.MouseX, e.MouseY, button)
			case tb.EventKey:
				gb.it.Keyboard(imterm.Key(e.Key), e.Ch)
			}
			gb.Refresh()
		}
	}()
	gb.Refresh()
	for range gb.refresh {
		gb.Term.Resize(gb.neww, gb.newh)
		gb.it.Start()

		if gb.B == nil {
			gb.gamewidth = gb.it.Input(10, 3, "Width", gb.gamewidth)
			gb.it.SameLine()
			gb.gameheight = gb.it.Input(10, 3, "Height", gb.gameheight)
			gb.gamemines = gb.it.Input(20, 3, "Mines", gb.gamemines)
			if gb.it.Button(20, 3, "Start") {
				w, err := strconv.Atoi(gb.gamewidth)
				if err != nil {
					gb.it.Finish()
					continue
				}
				h, err := strconv.Atoi(gb.gameheight)
				if err != nil {
					gb.it.Finish()
					continue
				}
				mines, err := strconv.Atoi(gb.gamemines)
				if err != nil {
					gb.it.Finish()
					continue
				}
				buf := make([]byte, 8)
				rand.Read(buf)
				seed := int64(0)
				for l1 := 0; l1 < 8; l1++ {
					seed <<= 8
					seed += int64(buf[l1])
				}
				r := mrand.New(mrand.NewSource(seed))
				gb.B = NewBoard(w, h, mines, r)
				gb.it.ClearState()
			}
		} else {
			width := gb.B.Width + 2
			if width+20 > gb.it.TermW {
				width = gb.it.TermW - 20
			}
			height := gb.B.Height + 2
			if height > gb.it.TermH {
				height = gb.it.TermH
			}

			gb.it.StartColumns(width)
			x, y, click := gb.it.Buffer(width, height, "Board", gb.B.Render())
			if click == imterm.MouseLeft {
				gb.B.Reveal(x, y)
				gb.Refresh()
			}
			if click == imterm.MouseRight {
				gb.B.Flag(x, y)
				gb.Refresh()
			}
			gb.it.NextColumn(20)
			switch gb.B.State {
			case Won:
				gb.it.Text(20, 3, "", "You Won!")
				if gb.it.Button(20, 3, "Leave") {
					gb.B = nil
					gb.it.ClearState()
					gb.Refresh()
				}
			case Lost:
				gb.it.Text(20, 3, "", "You Lost")
				if gb.it.Button(20, 3, "Leave") {
					gb.B = nil
					gb.it.ClearState()
					gb.Refresh()
				}
			default:
				gb.it.Text(20, 3, "Mines", fmt.Sprintf("%d", gb.B.Mines))
				gb.it.Text(20, 3, "Flags", fmt.Sprintf("%d", gb.B.GetFlags()))
			}
			gb.it.FinishColumns()
		}

		gb.it.Finish()
	}
}

func (gb *GameBoard) Refresh() {
	select {
	case gb.refresh <- struct{}{}:
	default:
	}
}
