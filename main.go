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

	st.Handler = NewGameRunner

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

type GameRunner struct {
	Term *tb.Termbox
	B    *Board
	it   *imterm.Imterm

	neww, newh int

	refresh chan struct{}

	gamewidth  string
	gameheight string
	gamemines  string
	username   string
	gameover   bool
}

func NewGameRunner(term *tb.Termbox, sshConn *ssh.ServerConn) sshterm.Term {
	term.SetInputMode(tb.InputEsc | tb.InputMouse)
	log.Printf("%s connected", sshConn.User())
	it, _ := imterm.New(&TermAdapter{term})
	gr := &GameRunner{
		Term:    term,
		refresh: make(chan struct{}, 2),
		it:      it,
		neww:    80,
		newh:    40,

		gamewidth:  "20",
		gameheight: "20",
		gamemines:  "30",
		username:   sshConn.User(),
	}

	go gr.Run()

	return gr
}

func (gr *GameRunner) Resize(w, h int) {
	if gr.neww != w || gr.newh != h {
		gr.neww, gr.newh = w, h
		gr.Refresh()
	}
}

func (gr *GameRunner) Run() {
	go func() {
		for {
			e := gr.Term.PollEvent()
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
				gr.it.Mouse(e.MouseX, e.MouseY, button)
			case tb.EventKey:
				gr.it.Keyboard(imterm.Key(e.Key), e.Ch)
			}
			gr.Refresh()
		}
	}()
	gr.Refresh()
	for range gr.refresh {
		gr.Term.Resize(gr.neww, gr.newh)
		gr.it.Start()
		if gr.B == nil {
			gr.gamewidth = gr.it.Input(10, 3, "Width", gr.gamewidth)
			gr.it.SameLine()
			gr.gameheight = gr.it.Input(10, 3, "Height", gr.gameheight)
			gr.gamemines = gr.it.Input(20, 3, "Mines", gr.gamemines)
			if gr.it.Button(20, 3, "Start") {
				w, err := strconv.Atoi(gr.gamewidth)
				if err != nil {
					gr.it.Finish()
					continue
				}
				h, err := strconv.Atoi(gr.gameheight)
				if err != nil {
					gr.it.Finish()
					continue
				}
				mines, err := strconv.Atoi(gr.gamemines)
				if err != nil {
					gr.it.Finish()
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
				gr.B = NewBoard(w, h, mines, r)
				log.Printf("%s started a game: %d x %d, %d mines", w, h, mines)
				gr.gameover = false
				gr.it.ClearState()
			}
		} else {
			width := gr.B.Width + 2
			if width+20 > gr.it.TermW {
				width = gr.it.TermW - 20
			}
			height := gr.B.Height + 2
			if height > gr.it.TermH {
				height = gr.it.TermH
			}

			gr.it.StartColumns(width)
			x, y, click := gr.it.Buffer(width, height, "Board", gr.B.Render())
			if click == imterm.MouseLeft {
				gr.B.Reveal(x, y)
				gr.Refresh()
			}
			if click == imterm.MouseRight {
				gr.B.Flag(x, y)
				gr.Refresh()
			}
			gr.it.NextColumn(20)
			switch gr.B.State {
			case Won:
				gr.it.Text(20, 3, "", "You Won!")
				if !gr.gameover {
					log.Printf("%s won a game!")
					gr.gameover = true
				}
				if gr.it.Button(20, 3, "Leave") {
					gr.B = nil
					gr.it.ClearState()
					gr.Refresh()
				}
			case Lost:
				gr.it.Text(20, 3, "", "You Lost")
				if !gr.gameover {
					log.Printf("%s lost a game!")
					gr.gameover = true
				}
				if gr.it.Button(20, 3, "Leave") {
					gr.B = nil
					gr.it.ClearState()
					gr.Refresh()
				}
			default:
				gr.it.Text(20, 3, "Mines", fmt.Sprintf("%d", gr.B.Mines))
				gr.it.Text(20, 3, "Flags", fmt.Sprintf("%d", gr.B.GetFlags()))
			}
			gr.it.FinishColumns()
		}

		gr.it.Finish()
	}
}

func (gr *GameRunner) Refresh() {
	select {
	case gr.refresh <- struct{}{}:
	default:
	}
}
