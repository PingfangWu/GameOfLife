package gol

import (
	"strconv"
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

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	h := p.ImageHeight
	w := p.ImageWidth
	filename := strconv.Itoa(w) + "x" + strconv.Itoa(h) //input filename???
	c.ioCommand <- ioInput
	c.ioFilename <- filename

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
		}
	}

	//A new world to store new state
	//newWorld := make([][]byte, p.ImageWidth)
	//for i := 0; i < p.ImageHeight; i++ {
	//	newWorld[i] = make([]byte, p.ImageHeight)
	//}

	worldHeight := len(world)

	turn := 0
	for ; turn < p.Turns; turn++ {
		if p.Threads == 1 {
			world = CalculateNextState(world, p)
		} else {
			newWorld := make([][]byte, 0)
			threads := p.Threads
			workerHeight := worldHeight / p.Threads
			workerSlice := make([]int, p.Threads)
			num := 1
			for i := 0; i < p.Threads; i++ {
				workerSlice[i] = workerHeight * num
				num++
			}
			channels := make([]chan [][]byte, p.Threads)
			for i := range channels {
				channels[i] = make(chan [][]byte, workerHeight*w)
			}

			for i := 0; i < threads; i++ {
				go worker(world, p, workerSlice[i-1], workerSlice[i], channels[i])
				if i == 0 {
					go worker(world, p, 0, workerSlice[i], channels[i])
				} else {
					if i == threads-1 {
						go worker(world, p, workerSlice[i-1], worldHeight, channels[i])
					}
				}
			}
			for i := 0; i < threads; i++ {
				newWorld = append(newWorld, <-channels[i]...)
			}
			//fmt.Println(len(newWorld))
			world = newWorld
		}

		// TODO: Report the final state using FinalTurnCompleteEvent.
	}

	alive := calculateAliveCells(world)
	finalTurn := FinalTurnComplete{CompletedTurns: turn, Alive: alive}

	c.events <- finalTurn
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
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

func CalculateNextState(world [][]byte, p Params) [][]byte {
	const dead, alive = 0, 255
	newState := make([][]byte, p.ImageWidth)
	for i := 0; i < p.ImageHeight; i++ {
		newState[i] = make([]byte, p.ImageWidth)
	}
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			state := world[y][x]
			neighbour := CalculateAroundNeighbours(x, y, p, world)
			if state == alive && neighbour < 2 {
				newState[y][x] = dead
			} else if (state == alive && neighbour == 2) || (state == 255 && neighbour == 3) {
				newState[y][x] = alive
			} else if state == alive && neighbour > 3 {
				newState[y][x] = dead
			} else if state == dead && neighbour == 3 {
				newState[y][x] = alive

			}
		}
	}

	return newState
}

func worker(world [][]byte, p Params, start int, end int, out chan<- [][]byte) {
	workerHeight := end - start
	partialWorld := make([][]byte, workerHeight) //each workerWorld
	for j := range partialWorld {
		partialWorld[j] = make([]byte, p.ImageWidth)
	}
	partialWorld = CalculateNextState(world, p)
	out <- partialWorld
}
