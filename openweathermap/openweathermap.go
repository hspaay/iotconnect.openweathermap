// Package openweathermap demonstrates building a simple IoTConnect publisher for weather forecasts
// This publishes the current weather for the cities
package openweathermap

import (
	"fmt"
	"time"

	"github.com/hspaay/iotconnect.golang/publisher"
	"github.com/hspaay/iotconnect.golang/standard"
)

// CurrentWeatherInst instance name for current weather
var CurrentWeatherInst = "current"

// LastHourWeatherInst instance name for last 1 hour weather (eg rain, snow)
var LastHourWeatherInst = "hour"

// ForecastWeatherInst instance name for upcoming forecast
var ForecastWeatherInst = "forecast"

// PublisherID default value. Can be overridden in config.
const PublisherID = "openweathermap"

// KelvinToC is nr of Kelvins at 0 degrees. openweathermap reports temp in Kelvin
// const KelvinToC = 273.1 // Kelvin at 0 celcius

// var weatherPub *publisher.PublisherState

// WeatherApp with application state, loaded from openweathermap.conf
type WeatherApp struct {
	Cities      []string `yaml:"cities"`
	APIKey      string   `yaml:"apikey"`
	PublisherID string   `yaml:"publisher"`
}

// PublishNodes creates the nodes and outputs
func (weatherApp *WeatherApp) PublishNodes(weatherPub *publisher.PublisherState) {
	pubNode := weatherPub.PublisherNode
	zone := pubNode.Zone
	outputs := weatherPub.Outputs

	// Create a node for each city with temperature outputs
	for _, city := range weatherApp.Cities {
		cityNode := standard.NewNode(zone, weatherApp.PublisherID, city)
		weatherPub.Nodes.UpdateNode(cityNode)

		lc := standard.NewConfig("language", standard.DataTypeEnum, "Reporting language. See https://openweathermap.org/current for more options", "en")
		weatherPub.Nodes.UpdateNodeConfig(cityNode, lc)

		// Add individual outputs for each weather info type
		outputs.NewOutput(cityNode, standard.IOTypeWeather, CurrentWeatherInst)
		outputs.NewOutput(cityNode, standard.IOTypeTemperature, CurrentWeatherInst)
		outputs.NewOutput(cityNode, standard.IOTypeHumidity, CurrentWeatherInst)
		outputs.NewOutput(cityNode, standard.IOTypeAtmosphericPressure, CurrentWeatherInst)
		outputs.NewOutput(cityNode, standard.IOTypeWindHeading, CurrentWeatherInst)
		outputs.NewOutput(cityNode, standard.IOTypeWindSpeed, CurrentWeatherInst)
		outputs.NewOutput(cityNode, standard.IOTypeRain, LastHourWeatherInst)
		outputs.NewOutput(cityNode, standard.IOTypeSnow, LastHourWeatherInst)

		// todo: Add outputs for various forecasts. This needs a paid account so maybe some other time.
		outputs.NewOutput(cityNode, standard.IOTypeWeather, ForecastWeatherInst)
		outputs.NewOutput(cityNode, standard.IOTypeTemperature, "max")
		outputs.NewOutput(cityNode, standard.IOTypeAtmosphericPressure, "min")
	}
}

// UpdateWeather obtains the weather and publishes the output value
// node:city -
//             type: weather    - instance: current, message: value
//             type: temperature- instance: current, message: value
//             type: humidity   - instance: current, message: value
//             etc...
// The iotconnect library will automatically publish changes to the values
func (weatherApp *WeatherApp) UpdateWeather(weatherPub *publisher.PublisherState) {
	// pubNode := weatherPub.GetNodeByID(standard.PublisherNodeID)
	apikey := weatherApp.APIKey
	outputHistory := weatherPub.OutputHistory
	weatherPub.Logger.Info("UpdateWeather")

	// publish the current weather for each of the city nodes
	for _, node := range weatherPub.Nodes.GetAllNodes() {
		if node.ID != standard.PublisherNodeID {
			language := node.Config["language"].Value
			currentWeather, err := GetCurrentWeather(apikey, node.ID, language)
			if err != nil {
				weatherPub.SetErrorStatus(node, "Current weather not available")
				return
			}
			var weatherDescription string = ""
			if len(currentWeather.Weather) > 0 {
				weatherDescription = currentWeather.Weather[0].Description
			}
			outputHistory.UpdateOutputValue(node, standard.IOTypeWeather, CurrentWeatherInst, weatherDescription)
			outputHistory.UpdateOutputValue(node, standard.IOTypeTemperature, CurrentWeatherInst, fmt.Sprintf("%.1f", currentWeather.Main.Temperature))
			outputHistory.UpdateOutputValue(node, standard.IOTypeHumidity, CurrentWeatherInst, fmt.Sprintf("%d", currentWeather.Main.Humidity))
			outputHistory.UpdateOutputValue(node, standard.IOTypeAtmosphericPressure, CurrentWeatherInst, fmt.Sprintf("%.0f", currentWeather.Main.Pressure))
			outputHistory.UpdateOutputValue(node, standard.IOTypeWindSpeed, CurrentWeatherInst, fmt.Sprintf("%.1f", currentWeather.Wind.Speed))
			outputHistory.UpdateOutputValue(node, standard.IOTypeWindHeading, CurrentWeatherInst, fmt.Sprintf("%.0f", currentWeather.Wind.Heading))
			outputHistory.UpdateOutputValue(node, standard.IOTypeRain, LastHourWeatherInst, fmt.Sprintf("%.1f", currentWeather.Rain.LastHour*1000))
			outputHistory.UpdateOutputValue(node, standard.IOTypeSnow, LastHourWeatherInst, fmt.Sprintf("%.1f", currentWeather.Snow.LastHour*1000))
		}
	}

	// TODO: move to its own 6 hour interval
	// weatherApp.UpdateForecast(weatherPub)
}

// UpdateForecast obtains a daily forecast and publishes this as a $forecast command
// This is published as follows: zone/publisher/node=city/$forecast/{type}/{instance}
//
// Note this requires a paid account - untested
func (weatherApp *WeatherApp) UpdateForecast(weatherPub *publisher.PublisherState) {
	apikey := weatherApp.APIKey

	// publish the daily forecast weather for each of the city nodes
	for _, node := range weatherPub.Nodes.GetAllNodes() {
		if node.ID != standard.PublisherNodeID {
			language := node.Config["language"].Value
			dailyForecast, err := GetDailyForecast(apikey, node.ID, language)
			if err != nil {
				weatherPub.SetErrorStatus(node, "Error getting the daily forecast")
				return
			} else if dailyForecast.List == nil {
				weatherPub.SetErrorStatus(node, "Daily forecast not provided")
				return
			}
			// build forecast history lists of weather and temperature forecasts
			// TODO: can this be done as a future history publication instead?
			weatherList := make(standard.HistoryList, 0)
			maxTempList := make(standard.HistoryList, 0)
			minTempList := make(standard.HistoryList, 0)

			for _, forecast := range dailyForecast.List {
				timestamp := time.Unix(int64(forecast.Date), 0)

				// add the weather descriptions
				var weatherDescription string = ""
				if len(forecast.Weather) > 0 {
					weatherDescription = forecast.Weather[0].Description
				}
				weatherList = append(weatherList, &standard.HistoryValue{Timestamp: timestamp, Value: weatherDescription})
				maxTempList = append(maxTempList, &standard.HistoryValue{Timestamp: timestamp, Value: fmt.Sprintf("%.1f", forecast.Temp.Max)})
				minTempList = append(maxTempList, &standard.HistoryValue{Timestamp: timestamp, Value: fmt.Sprintf("%.1f", forecast.Temp.Min)})
			}
			weatherPub.UpdateForecast(node, standard.IOTypeWeather, ForecastWeatherInst, weatherList)
			weatherPub.UpdateForecast(node, standard.IOTypeTemperature, "max", maxTempList)
			weatherPub.UpdateForecast(node, standard.IOTypeTemperature, "min", minTempList)

		}
	}
}

// OnNodeConfigHandler handles requests to update node configuration
func (weatherApp *WeatherApp) OnNodeConfigHandler(node *standard.Node, config standard.AttrMap) standard.AttrMap {
	return nil
}

// NewWeatherApp creates the weather app
func NewWeatherApp() *WeatherApp {
	app := WeatherApp{
		Cities:      make([]string, 0),
		PublisherID: PublisherID,
	}
	return &app
}