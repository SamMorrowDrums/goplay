package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func main() {
	mw := multiWeatherProvider{
		openWeatherMap{},
		weatherUnderground{apiKey: "73e2e205cb8f0a0e"},
		forecastio{apiKey: "fc07393533d1a776b560c5953726f3a6"},
	}

	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		city := strings.SplitN(r.URL.Path, "/", 3)[2]

		temp, err := mw.temperature(city)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"city": city,
			"temp": temp,
			"took": time.Since(begin).String(),
		})

	})

	http.HandleFunc("/location/", func(w http.ResponseWriter, r *http.Request) {
		city := strings.SplitN(r.URL.Path, "/", 3)[2]

		location, err := location(city)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"city":      city,
			"longitude": location.Longitude,
			"latitude":  location.Latitude,
		})
	})

	http.ListenAndServe(":8080", nil)
}

type geometry struct {
	Geometry struct {
		Location struct {
			Longitude float64 `json:"lng"`
			Latitude  float64 `json:"lat"`
		} `json:"location"`
	} `json:"geometry"`
}

type googleResponse struct {
	Results []geometry `json:"results"`
}

type locationData struct {
	Longitude float64
	Latitude  float64
}

func location(city string) (locationData, error) {
	resp, err := http.Get("https://maps.googleapis.com/maps/api/geocode/json?address=" + city)
	if err != nil {
		return locationData{}, err
	}

	defer resp.Body.Close()

	var g googleResponse
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return locationData{}, err
	}

	var d locationData
	d.Latitude = g.Results[0].Geometry.Location.Latitude
	d.Longitude = g.Results[0].Geometry.Location.Longitude

	return d, nil
}

type weatherProvider interface {
	temperature(city string) (float64, error)
}

type multiWeatherProvider []weatherProvider

func (w multiWeatherProvider) temperature(city string) (float64, error) {

	temps := make(chan float64, len(w))
	errs := make(chan error, len(w))

	for _, provider := range w {
		go func(p weatherProvider) {
			k, err := p.temperature(city)
			if err != nil {
				errs <- err
				return
			}
			temps <- k
		}(provider)
	}

	sum := 0.0

	for i := 0; i < len(w); i++ {
		select {
		case temp := <-temps:
			sum += temp
		case err := <-errs:
			return 0, err
		}
	}
	return sum / float64(len(w)), nil
}

type openWeatherMap struct{}

func (w openWeatherMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?q=" + city)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var d struct {
		Main struct {
			Kelvin float64 `json:"temp"`
		} `json:"main"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	log.Printf("openWeatherMap: %s: %.2f", city, d.Main.Kelvin)
	return d.Main.Kelvin, nil
}

type place struct{}

type weatherUnderground struct {
	apiKey string
}

func (w weatherUnderground) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.wunderground.com/api/" + w.apiKey + "/conditions/q/uk/" + city + ".json")
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var d struct {
		Observation struct {
			Celcsius float64 `json:"temp_c"`
		} `json:"current_observation"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	kelvin := d.Observation.Celcsius + 273.15
	log.Printf("weatherUnderground: %s: %.2f", city, kelvin)
	return kelvin, nil
}

type forecastio struct {
	apiKey string
}

func (w forecastio) temperature(city string) (float64, error) {

	location, err := location(city)
	if err != nil {
		return 0, err
	}

	lat := strconv.FormatFloat(location.Latitude, 'f', 6, 64)
	long := strconv.FormatFloat(location.Longitude, 'f', 6, 64)
	resp, err := http.Get("https://api.forecast.io/forecast/" + w.apiKey + "/" + lat + "," + long)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var d struct {
		Observation struct {
			Fahrenheit float64 `json:"temperature"`
		} `json:"currently"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	kelvin := ((d.Observation.Fahrenheit - 32) * 5 / 9) + 273.15
	log.Printf("forecastIO: %s: %.2f", city, kelvin)
	return kelvin, nil
}
