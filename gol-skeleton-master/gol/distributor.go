package gol

import (
	"fmt"
	"github.com/veandco/go-sdl2/sdl"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

const (
	noAction    = 0
	pause       = 1
	save        = 2
	quitAndSave = 3
)

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	h := p.ImageHeight
	w := p.ImageWidth
	c.ioCommand <- ioInput
	//filename := strconv.Itoa(w) + "x" + strconv.Itoa(h) //input filename???
	filename := fmt.Sprintf("%dx%d", h, w)
	c.ioFilename <- filename

	actionChannel := make(chan int)
	resumeChannel := make(chan bool)
	turnChannel := make(chan int)

	// TODO: Create a 2D slice to store the world.
	//old world for the original state
	world := make([][]byte, p.ImageHeight) //建立2D slice for old world
	for i := 0; i < p.ImageHeight; i++ {   //create rows by columns
		world[i] = make([]byte, p.ImageWidth) //empty 2D slice with nothing in it ***一样
	}
	//Nested loop: iterate over every single column and row
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			world[i][j] = <-c.ioInput

			if world[i][j] == 1 {
				c.events <- CellFlipped{
					Cell:           util.Cell{j, i},
					CompletedTurns: 0,
				}
			}
		}
	}

	//A new world to store new state
	//newWorld := make([][]byte, p.ImageWidth)
	//for i := 0; i < p.ImageHeight; i++ {
	//	newWorld[i] = make([]byte, p.ImageHeight)
	//}

	worldHeight := len(world)

	turn := 0
	ticker := time.Tick(2 * time.Second)
	go dealWithKey(keyPresses, turnChannel, actionChannel, resumeChannel)

	for ; turn < p.Turns; turn++ {
		if p.Threads == 1 {
		} else {
			newWorld := make([][]byte, 0)
			threads := p.Threads
			workerHeight := worldHeight / p.Threads
			channels := make([]chan [][]byte, p.Threads)
			// for i := range channels {
			// 	channels[i] = make(chan [][]byte, workerHeight*w)
			// }

			i := 0
			for ; i < threads-1; i++ {
				channels[i] = make(chan [][]byte, workerHeight*w)
				go worker(world, p, i*workerHeight, (i+1)*workerHeight, channels[i], c, turn)
			}
			last_height := worldHeight - i*workerHeight
			channels[i] = make(chan [][]byte, last_height*w)
			go worker(world, p, i*workerHeight, worldHeight, channels[i], c, turn)
			for i := 0; i < threads; i++ {
				newWorld = append(newWorld, <-channels[i]...)
			}

			requestedAction := actOrReturn(actionChannel)
			resume := true
			if requestedAction == pause || requestedAction == save {
				turnChannel <- turn
				resume = <-resumeChannel
			}
			if requestedAction == quitAndSave || resume == false {
				quit(world, c, filename, turn)
			}

			if turn == p.Turns {
				quit(world, c, filename, turn)
			}

			func quit(world [][]byte, c distributorChannels, filename string, turn int) {
				alive := calculateAliveCells(world)
				finalTurn := FinalTurnComplete{CompletedTurns: turn, Alive: alive}
			}

				reportAliveCells(world, ticker, c, turn)
			world = CalculateNextState(world, 0, worldHeight, p)
			complete := TurnComplete{CompletedTurns: turn}
			c.events <- complete
			finalTurn := FinalTurnComplete{CompletedTurns: turn, Alive: alive}

			c.events <- finalTurn

			func reportAliveCells(world[][]byte, ticker<-chan time.Time, c distributorChannels, turn int) {
				//	select {
				//	case <-ticker:
				//		aliveCells := len(calculateAliveCells(world))
				//
				//		c.events <- AliveCellsCount{
				//			CellsCount:     aliveCells,
				//			CompletedTurns: turn,
				//		}
				//	default:
				//		return
				//	}
				//}
				actOnAction := actOrReturn(action)
				switch actOnAction {
				case 1:
					turnChannel <- turn
					<-resumeChannel
				}
			}
			world = newWorld
			complete := TurnComplete{CompletedTurns: turn}
			c.events <- complete
			//fmt.Println(len(newWorld))

		}

		// TODO: Report the final state using FinalTurnCompleteEvent.
	}
	if turn == p.Turns {
		alive := calculateAliveCells(world)

		// Make sure that the Io has finished any output before exiting.

		c.ioCommand <- ioOutput
		filename = filename + fmt.Sprintf("x%v", turn)
		c.ioFilename <- filename
		for i := range world {
			for j := range world[i] {
				c.ioOutput <- world[i][j]
			}
		}
		c.ioCommand <- ioCheckIdle
		<-c.ioIdle

		c.events <- StateChange{turn, Quitting}

		// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
		close(c.events)
	}
}
func actOrReturn(action chan int) int {
	select {
	case <-action:
		return 1
	default:
		return 0
	}
}
func quit(c distributorChannels) {
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	close(c.events)
}
func screenShot(world [][]byte, c distributorChannels, filename string, turn int) {
	c.ioCommand <- ioOutput
	filename = filename + fmt.Sprintf("x%v", turn)
	c.ioFilename <- filename
}
func dealWithKey(turn int, c distributorChannels, keyPresses <-chan rune, world [][]byte, filename string, turnRequest, action chan int, returned chan bool) {
	for {
		select {
		case key := <-keyPresses:
			switch key {
			case sdl.K_q:
				quit(c)
				returned <- false
			case sdl.K_s:
				screenShot(world, c, filename, turn)
				quit(c)
				action <- 0
				returned <- false
			case sdl.K_p:
				action <- 1
				fmt.Println(<-turnRequest)
				pKey(turn, c, keyPresses, world, filename, action, returned)
			}
		}
	}
}
func pKey(turn int, c distributorChannels, keyPresses <-chan rune, world [][]byte, filename string, action chan int, returned chan bool) {
	select {
	case key := <-keyPresses:
		switch key {
		case sdl.K_q:
			quit(c)
			//action <-0
			returned <- false
		case sdl.K_s:
			screenShot(world, c, filename, turn)
			quit(c)
			//action <-0
			returned <- false
		case sdl.K_p:
			fmt.Println("Continuing")
			//action <-0
			returned <- true
		}
	}
}




// TODO: Execute all turns of the Game of Life.
// Iterate gameoflife algorithm --- using for loop
// in each iteration of the for loop ---> update the state of the game of life

func getStateValue(x int, y int, p Params, world [][]byte) int { // infinite wraparound world
	if x < 0 {
		x = p.ImageWidth - 1
	} else if x >= p.ImageWidth {
		x = 0
	}
	if y < 0 {
		y = p.ImageHeight - 1
	} else if y >= p.ImageHeight {
		y = 0
	}
	value := int(world[y][x])
	if value == 255 {
		return 1
	}
	return 0
}

func CalculateAroundNeighbours(x int, y int, p Params, world [][]byte) int { //Add up all the neighbors
	neighbours := 0
	neighbours += getStateValue(x-1, y-1, p, world)
	neighbours += getStateValue(x-1, y, p, world)
	neighbours += getStateValue(x-1, y+1, p, world)
	neighbours += getStateValue(x, y-1, p, world)
	neighbours += getStateValue(x, y+1, p, world)
	neighbours += getStateValue(x+1, y-1, p, world)
	neighbours += getStateValue(x+1, y, p, world)
	neighbours += getStateValue(x+1, y+1, p, world)
	return neighbours
}

func getNewState(x int, y int, p Params, world [][]byte, c distributorChannels, turn int) byte {
	const dead, alive = 0, 255
	state := getStateValue(x, y, p, world)
	neighbour := CalculateAroundNeighbours(x, y, p, world)
	if state == 1 && neighbour < 2 {
		c.events <- CellFlipped{
			Cell:           util.Cell{X: y, Y: x},
			CompletedTurns: turn,
		}
		return dead
	} else if (state == 1 && neighbour == 2) || (state == 1 && neighbour == 3) {
		return alive
	} else if state == 1 && neighbour > 3 {
		c.events <- CellFlipped{
			Cell:           util.Cell{X: y, Y: x},
			CompletedTurns: turn,
		}
		return dead
	} else if state == 0 && neighbour == 3 {
		c.events <- CellFlipped{
			Cell:           util.Cell{X: y, Y: x},
			CompletedTurns: turn,
		}
		return alive

	} else {
		return dead
	}

}
func CalculateNextState(world [][]byte, startY, endY int, p Params, c distributorChannels, turn int) [][]byte {
	height := endY - startY
	newState := make([][]byte, height)
	// for i := range height {
	// 	newState[i] = make([]byte, p.ImageWidth)
	// 	// for j := range newState {
	// 	// 	newState[i][j] = world[i+startY][j]
	// 	// }
	// }
	for y := 0; y < height; y++ {
		newState[y] = make([]byte, p.ImageWidth)
		for x := 0; x < p.ImageWidth; x++ {
			state := getNewState(x, y+startY, p, world, c, turn)
			newState[y][x] = state
		}
	}

	return newState
}

func worker(world [][]byte, p Params, startY, endY int, out chan<- [][]byte, c distributorChannels, turn int) {
	partialWorld := CalculateNextState(world, startY, endY,p, c, turn)
	out <- partialWorld
}

func calculateAliveCells(world [][]byte) []util.Cell {
	cells := make([]util.Cell, 0)
	for i := range world {
		for j := range world[i] {
			if world[i][j] == 255 {
				cells = append(cells, util.Cell{j, i})
			}
		}
	}
	return cells
}
