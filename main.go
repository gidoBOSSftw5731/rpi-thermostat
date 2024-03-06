package main

import (
	"flag"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/d2r2/go-dht"
	weather "github.com/gidoBOSSftw5731/goweather"
	"github.com/gidoBOSSftw5731/log"
	"github.com/warthog618/gpiod"
)

const (
	// sensor_pin is the pin of the sensor
	sensor_pin  = 18
	sensor_type = dht.DHT11

	relay_pin = 4

	// desired_temp is the temperature we want to maintain (assuming heating)
	desired_temp = 19

	// histeresis is the range of temperature we allow, so the range is +- histeresis
	histeresis = 1

	// outside_temp_threshold is the temperature above which we want to turn on the AC
	outside_temp_threshold = 15.56
)

var (
	relay_line *gpiod.Line

	currentClimateData climateData

	//relayLockTime is a timestamp of, if the relay is locked, when the lock will be lifted.
	// locks are manual overrides given by the user.
	relayLockTime time.Time

	// active is a boolean that is true if the relay is active
	active bool

	// current zip code set by flag that defaults to 90210
	zipCode = flag.String("zip", "90210", "The zip code to get the weather for to determine if heat or AC should be on.")

	// apiKey is the api key for the openweathermap api
	apiKey = flag.String("api", "", "The api key for the openweathermap api.")
)

type climateData struct {
	Temperature        float32
	Humidity           float32
	OutsideTemperature float32
	OutsideHumidity    float32
	IsACOn             bool
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

	// also get the outside temperature
	w, err := weather.CurrentWeather(*zipCode, *apiKey)
	if err != nil {
		log.Errorln("Error getting weather: ", err)
		return temperature, humidity
	}
	currentClimateData.OutsideTemperature = float32(w.Main.Temp)
	currentClimateData.OutsideHumidity = float32(w.Main.Humidity)

	return temperature, humidity
}

// checker repeatedly checks if the temperature is within the
// desired range and turns the relay on or off accordingly
func checker() {
	for {
		temperature := checkTemp()
		switch {
		case checkOutsideTemp(outside_temp_threshold):
			// turn on the relay
			currentClimateData.IsACOn = true
			setRelay(1)
		case temperature <= desired_temp-histeresis:
			// turn on the relay
			currentClimateData.IsACOn = false
			setRelay(1)
		case temperature > desired_temp+histeresis:
			// turn off the relay
			currentClimateData.IsACOn = false
			setRelay(0)
		}
	}
}

// checkOutsideTemp checks the outside temperature and, if it's higher than the threshold set, returns true to turn on the AC
func checkOutsideTemp(threshold float64) bool {
	// get the outside temperature
	w, err := weather.CurrentWeather(*zipCode, *apiKey)
	if err != nil {
		log.Errorln("Error getting weather: ", err)
		return false
	}

	log.Tracef("Weather data: %+v\n", w)

	// if the temperature is higher than the threshold, return true]
	return w.Main.Temp > threshold
}

func setRelay(state int) {
	if time.Now().Before(relayLockTime) {
		log.Traceln("Relay is locked, not setting relay to", state)
		return
	}
	// set gpio pin to state
	log.Traceln("Setting relay to", state)
	active = state == 1
	err := relay_line.SetValue(state)
	if err != nil {
		log.Errorln("Error setting relay: ", err)
	}
}

func main() {
	log.SetCallDepth(4)

	flag.Parse()

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
	http.HandleFunc("/lock/", lockHandler)
	http.HandleFunc("/style.css", func(resp http.ResponseWriter, req *http.Request) {
		http.ServeFile(resp, req, "style.css")
	})
	http.HandleFunc("/isactive", func(resp http.ResponseWriter, req *http.Request) {
		resp.Write([]byte(strconv.FormatBool(active)))
	})
	http.HandleFunc("/islocked", func(resp http.ResponseWriter, req *http.Request) {
		resp.Write([]byte(strconv.FormatBool(time.Now().Before(relayLockTime))))
	})
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

	switch len(URLSplit) {
	case 4:
		locker(resp, req, URLSplit)
	default:
		resp.WriteHeader(http.StatusBadRequest)
	}
}

func locker(resp http.ResponseWriter, req *http.Request, URLSplit []string) {
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
	relayLockTime = time.Unix(0, 0)
	// set the relay
	setRelay(state)
	// set the lock time
	relayLockTime = time.Now().Add(time.Duration(mins) * time.Minute)

	// redirect to the index page
	http.Redirect(resp, req, "/", http.StatusTemporaryRedirect)
}
