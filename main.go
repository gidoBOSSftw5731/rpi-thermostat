package main

import (
	"github.com/d2r2/go-dht"
	"github.com/gidoBOSSftw5731/log"
	"github.com/warthog618/gpiod"
)

const (
	// sensor_pin is the pin of the sensor
	sensor_pin  = 18
	sensor_type = dht.DHT11

	relay_pin = 4

	// desired_temp is the temperature we want to maintain (assuming heating)
	desired_temp = 18

	// histeresis is the range of temperature we allow, so the range is +- histeresis
	histeresis = 1
)

var (
	relay_line *gpiod.Line
)

func checkTemp() float32 {
	temperature, _, _, _ :=
		dht.ReadDHTxxWithRetry(sensor_type, sensor_pin, false, 50)

	return temperature
}

// checker repeatedly checks if the temperature is within the
// desired range and turns the relay on or off accordingly
func checker() {
	for {
		temperature := checkTemp()

		switch {
		case temperature < desired_temp-histeresis:
			// turn on the relay
			setRelay(1)
		case temperature > desired_temp+histeresis:
			// turn off the relay
			setRelay(0)
		}
	}
}

func setRelay(state int) {
	// set gpio pin to state
	log.Traceln("Setting relay to", state)
	relay_line.SetValue(state)
}

func main() {
	var err error
	relay_line, err = gpiod.RequestLine("gpiochip0", relay_pin, gpiod.AsOutput())
	if err != nil {
		panic(err)
	}
	defer relay_line.Close()

	go checker()

}
