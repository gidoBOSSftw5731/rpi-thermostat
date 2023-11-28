package main

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	currentClimateData climateData

	//relayLockTime is a timestamp of, if the relay is locked, when the lock will be lifted.
	// locks are manual overrides given by the user.
	relayLockTime time.Time
)

type climateData struct {
	Temperature float32
	Humidity    float32
}

func checkTemp() float32 {
	temperature, _ := checkTempAndHumidity()
	return temperature
}

func checkTempAndHumidity() (float32, float32) {
	temperature, humidity, _, _ :=
		dht.ReadDHTxxWithRetry(sensor_type, sensor_pin, false, 50)
	currentClimateData.Temperature = temperature
	currentClimateData.Humidity = humidity

	return temperature, humidity
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
	if time.Now().Before(relayLockTime) {
		log.Traceln("Relay is locked, not setting relay to", state)
		return
	}
	// set gpio pin to state
	log.Traceln("Setting relay to", state)
	relay_line.SetValue(state)
}

func main() {
	log.SetCallDepth(4)

	var err error
	relay_line, err = gpiod.RequestLine("gpiochip0", relay_pin, gpiod.AsOutput())
	if err != nil {
		panic(err)
	}
	defer relay_line.Close()

	go checker()

	// start listening on port 8080 with webserver
	startWebserver()

}

func startWebserver() {
	// start webserver
	log.Infoln("Starting webserver on port 8080")
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/lock", lockHandler)
	http.ListenAndServe("127.0.0.1:8080", nil)
}

func indexHandler(resp http.ResponseWriter, req *http.Request) {
	// get temperature and humidity

	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		panic(err)
	}

	err = tmpl.Execute(resp, currentClimateData)
	if err != nil {
		log.Errorln("Templating error: ", err)
		resp.WriteHeader(http.StatusInternalServerError)
	}
}

func lockHandler(resp http.ResponseWriter, req *http.Request) {
	// split path into parts
	URLSplit := strings.Split(req.URL.Path, "/")

	// check if the path has both a state and a time
	if len(URLSplit) != 4 {
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	var state int
	// parse the state
	switch URLSplit[2] {
	case "on":
		state = 1
	case "off":
		state = 0
	default:
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	// parse the time in minutes
	mins, err := strconv.Atoi(URLSplit[3])
	if err != nil {
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	//set the lock time to 0
	relayLockTime = time.Time{}
	// set the relay
	setRelay(state)
	// set the lock time
	relayLockTime = time.Now().Add(time.Duration(mins) * time.Minute)

}
