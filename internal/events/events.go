// Package events holds the messages that travel over RabbitMQ (README: Event flow). Their JSON forms are the wire
// format, and a position's is also the history row.
package events

import (
	"regexp"
	"time"

	"github.com/amirradjou/overhead/internal/feeds"
)

// Position is one new position report, as published on the positions exchange.
type Position struct {
	// Run names the poller process that published it; Seq counts its events from 1, so a missing number is a lost
	// message and a repeated one a duplicate.
	Run       string    `json:"run"`
	Seq       uint64    `json:"seq"`
	Published time.Time `json:"published"`
	feeds.Position
}

// runPattern is the form of a run id (relay.NewRun): start time and a random suffix. A run id ends up in file names,
// so consumers check it before trusting a message.
var runPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$`)

// ValidRun reports whether run is a well-formed run id.
func ValidRun(run string) bool { return runPattern.MatchString(run) }

// EnrichJob asks for what is known about an aircraft, and about the flight it is flying under Callsign.
type EnrichJob struct {
	Hex      string `json:"hex"`
	Callsign string `json:"callsign,omitempty"`
}

// Aircraft is what enrichment found about an airframe and its current flight, as published on the aircraft exchange.
// Missing parts are nil or empty: an unknown airframe, a callsign with no published route, no photo.
type Aircraft struct {
	Hex          string    `json:"hex"`
	Registration string    `json:"registration,omitempty"`
	TypeCode     string    `json:"type_code,omitempty"`
	TypeName     string    `json:"type_name,omitempty"`
	Manufacturer string    `json:"manufacturer,omitempty"`
	Owner        string    `json:"owner,omitempty"`
	OwnerCountry string    `json:"owner_country,omitempty"`
	Route        *Route    `json:"route,omitempty"`
	Photo        *Photo    `json:"photo,omitempty"`
	Looked       time.Time `json:"looked"`
}

// Route is the scheduled route of a callsign. adsbdb's route data may not be copied into other databases, so routes
// live only in enrichment's day-long cache and on the screen, never in the history.
type Route struct {
	Callsign    string  `json:"callsign"`
	FlightIATA  string  `json:"flight_iata,omitempty"`
	Airline     string  `json:"airline,omitempty"`
	AirlineICAO string  `json:"airline_icao,omitempty"`
	Origin      Airport `json:"origin"`
	Destination Airport `json:"destination"`
}

// Airport is one end of a route.
type Airport struct {
	ICAO    string  `json:"icao,omitempty"`
	IATA    string  `json:"iata,omitempty"`
	Name    string  `json:"name,omitempty"`
	City    string  `json:"city,omitempty"`
	Country string  `json:"country,omitempty"`
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
}

// Photo is a picture of the exact airframe. Planespotters' terms: show the photographer and link to the photo page.
type Photo struct {
	Thumbnail    string `json:"thumbnail"`
	Large        string `json:"large,omitempty"`
	Link         string `json:"link"`
	Photographer string `json:"photographer"`
}
