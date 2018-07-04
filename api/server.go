package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/ipchikin/shortest-path/codec"

	"github.com/ipchikin/shortest-path/types"
	"github.com/julienschmidt/httprouter"
)

type Server struct {
	DB     *badger.DB
	Client *http.Client
}

func GenTokenHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var data interface{}
	var locations [][2]string
	err := json.NewDecoder(r.Body).Decode(&locations)
	if err != nil {
		data = types.ErrorResponse{errors.New("Invalid Inputs")}
	} else if len(locations) < 2 {
		data = types.ErrorResponse{errors.New("Invalid Inputs")}
	} else {
		encryptionKey := []byte(os.Getenv("ENCRYPTION_KEY"))
		token, err := codec.GenerateToken(locations, encryptionKey)
		if err != nil {
			data = types.ErrorResponse{err}
		} else {
			data = types.TokenResponse{token}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) GetRouteHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var data interface{}
	token := ps.ByName("token")
	// generate key from token
	key, err := codec.GenerateKey(token)
	if err != nil {
		data = types.FailureResponse{"failure", errors.New("Invalid Token")}
	} else {
		// retrieve data from the store
		data, err = s.GetData(key)

		// call google map api if data not found
		if err != nil {
			encryptionKey := []byte(os.Getenv("ENCRYPTION_KEY"))
			// decrypt the key to generate inputs
			locations, err := codec.GenerateInputs(key, encryptionKey)
			if err != nil {
				data = types.FailureResponse{"failure", errors.New("Invalid Token")}
			} else {
				apiKey := os.Getenv("API_KEY")
				// generate Google Map api url
				url := GMapApiUrl(locations, apiKey)

				resp, err := CallGMapApi(s.Client, url, locations[0])
				if err != nil {
					data = types.FailureResponse{"failure", err}
				} else {
					data = resp
					// cache the data for 10 mins
					ttl := 10 * time.Minute
					// no error needed?
					s.SetData(key, resp, ttl)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(data)
}

func (s Server) GetData(key []byte) (types.SuccessResponse, error) {
	var data types.SuccessResponse
	err := s.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		val, err := item.Value()
		if err != nil {
			return err
		}
		data, err = codec.DecodeData(val)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return data, err
	}

	return data, nil
}

func (s *Server) SetData(key []byte, val types.SuccessResponse, ttl time.Duration) error {
	err := s.DB.Update(func(txn *badger.Txn) error {
		// encode the data for storing
		by, err := codec.EncodeData(val)
		if err != nil {
			return err
		}
		// store the data
		err = txn.SetWithTTL(key, by, ttl)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func GMapApiUrl(locations [][2]string, apiKey string) string {
	var url string
	if len(locations) == 2 {
		url = fmt.Sprintf("https://maps.googleapis.com/maps/api/directions/json?origin=%s,%s&destination=%s,%s&key=%s", locations[0][0], locations[0][1], locations[1][0], locations[1][1], apiKey)
	} else {
		// Combine the waypoints to a string for Google Map Directions API, lat1,lng1|lat2,lng2|...
		var wps []string
		for _, wp := range locations[1 : len(locations)-1] {
			wps = append(wps, strings.Join(wp[:], ","))
		}
		waypoints := strings.Join(wps, "|")
		url = fmt.Sprintf("https://maps.googleapis.com/maps/api/directions/json?origin=%s,%s&destination=%s,%s&waypoints=%s&key=%s", locations[0][0], locations[0][1], locations[len(locations)-1][0], locations[len(locations)-1][1], waypoints, apiKey)
	}

	return url
}

func CallGMapApi(client *http.Client, url string, start [2]string) (types.SuccessResponse, error) {
	var success types.SuccessResponse
	var m types.Message

	resp, err := client.Get(url)
	if err != nil {
		return success, err
	}

	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(&m)

	if len(m.Routes) < 1 {
		return success, errors.New("No Route Found")
	}

	path := [][2]string{start}
	distance := int64(0)
	t := int64(0)
	for _, leg := range m.Routes[0].Legs {
		distance += leg.Distance.Value
		t += leg.Duration.Value
		for _, step := range leg.Steps {
			path = append(path, [2]string{strconv.FormatFloat(step.EndLocation.Lat, 'f', -1, 64), strconv.FormatFloat(step.EndLocation.Lng, 'f', -1, 64)})
		}
	}

	success = types.SuccessResponse{"success", path, distance, t}
	return success, nil
}
