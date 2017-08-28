package main

import (
	"math/rand"

	"github.com/andyleap/imterm"
)

type GameState int

const (
	Start GameState = iota
	Playing
	Won
	Lost
)

type Board struct {
	Cells  [][]*Cell
	Width  int
	Height int
	Mines  int
	State  GameState
	r      *rand.Rand
}

type Cell struct {
	Mine     bool
	Flagged  bool
	Revealed bool
}

type Pos struct {
	x, y int
}

func NewBoard(w, h, mines int, r *rand.Rand) *Board {
	b := &Board{Width: w, Height: h, Mines: mines, r: r}

	b.Cells = make([][]*Cell, h)
	for i := range b.Cells {
		b.Cells[i] = make([]*Cell, w)
		for j := range b.Cells[i] {
			b.Cells[i][j] = &Cell{}
		}
	}

	return b
}

func (b *Board) init(x, y int) {
	cellsLeft := (b.Width * b.Height) - 1
	minesLeft := b.Mines
	mines := 0
	for i, row := range b.Cells {
		for j, cell := range row {
			if i == y && j == x {
				continue
			}
			if b.r.Intn(cellsLeft) < minesLeft {
				cell.Mine = true
				mines++
				minesLeft--
			}
			cellsLeft--
		}
	}
	b.Mines = mines
	b.State = Playing
}

func (b *Board) Get(x, y int) *Cell {
	return b.Cells[y][x]
}

func (b *Board) Neighbors(x, y int, corners bool, cb func(x, y int)) {
	if x > 0 {
		if y > 0 && corners {
			cb(x-1, y-1)
		}
		cb(x-1, y)
		if y < b.Height-1 && corners {
			cb(x-1, y+1)
		}
	}
	if y > 0 {
		cb(x, y-1)
	}
	if y < b.Height-1 {
		cb(x, y+1)
	}
	if x < b.Width-1 {
		if y > 0 && corners {
			cb(x+1, y-1)
		}
		cb(x+1, y)
		if y < b.Height-1 && corners {
			cb(x+1, y+1)
		}
	}
}

func (b *Board) GetNeighborCount(x, y int) int {
	if b.Get(x, y).Mine {
		return -1
	}
	nearby := 0
	b.Neighbors(x, y, true, func(x, y int) {
		if b.Get(x, y).Mine {
			nearby++
		}
	})
	return nearby
}

func (b *Board) Render() [][]imterm.Cell {
	buffer := [][]imterm.Cell{}
	for y, row := range b.Cells {
		bufrow := []imterm.Cell{}
		for x, cell := range row {
			bufcell := imterm.Cell{}
			bufcell.Bg = imterm.ColorBlue
			if cell.Flagged {
				bufcell.Char = '√'
			} else if cell.Revealed {
				count := b.GetNeighborCount(x, y)
				bufcell.Bg = imterm.ColorWhite
				bufcell.Fg = imterm.ColorBlack
				if count > 0 {
					bufcell.Char = rune(("0123456789")[count])
				}
			} else if cell.Mine && (b.State != Playing && b.State != Start) {
				bufcell.Char = '*'
			}
			bufrow = append(bufrow, bufcell)
		}
		buffer = append(buffer, bufrow)
	}
	return buffer
}

func (b *Board) Reveal(x, y int) {
	if b.State == Start {
		b.init(x, y)
	}
	if b.State != Playing {
		return
	}
	cell := b.Get(x, y)
	if cell.Flagged {
		return
	}
	if cell.Mine {
		b.State = Lost
		return
	}
	if cell.Revealed {
		return
	}
	cell.Revealed = true
	queue := []Pos{}
	queue = append(queue, Pos{x, y})
	for len(queue) > 0 {
		cur := queue[len(queue)-1]
		queue = queue[0 : len(queue)-1]
		b.Neighbors(cur.x, cur.y, false, func(x, y int) {
			ncell := b.Get(x, y)
			if !ncell.Revealed && !ncell.Mine {
				ncell.Revealed = true
				queue = append(queue, Pos{x, y})
			}
		})
	}
	b.checkWin()
	return
}

func (b *Board) checkWin() {
	left := 0
	for _, row := range b.Cells {
		for _, cell := range row {
			if !cell.Revealed {
				left++
			}
		}
	}
	if b.State == Playing && left == b.Mines {
		b.State = Won
	}
}

func (b *Board) GetFlags() (flags int) {
	for _, row := range b.Cells {
		for _, cell := range row {
			if cell.Flagged {
				flags++
			}
		}
	}
	return
}

func (b *Board) Flag(x, y int) {
	cell := b.Get(x, y)
	cell.Flagged = !cell.Flagged
}
