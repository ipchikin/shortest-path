package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
				urls := GMapApiUrls(locations, apiKey)

				resp, err := CallGMapApi(s.Client, urls, locations[0])
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

func GMapApiUrls(locations [][2]string, apiKey string) []string {
	var urls []string
	if len(locations) == 2 {
		urls = []string{fmt.Sprintf("https://maps.googleapis.com/maps/api/directions/json?origin=%s,%s&destination=%s,%s&key=%s", locations[0][0], locations[0][1], locations[1][0], locations[1][1], apiKey)}
	} else {
		permEnd := len(locations) - 2
		for perm := range GeneratePermutations(locations[1:]) {
			// Combine the waypoints to a string for Google Map Directions API, lat1,lng1|lat2,lng2|...
			var wps []string
			for _, wp := range perm[:permEnd] {
				wps = append(wps, strings.Join(wp[:], ","))
			}
			waypoints := strings.Join(wps, "|")
			urls = append(urls, fmt.Sprintf("https://maps.googleapis.com/maps/api/directions/json?origin=%s,%s&destination=%s,%s&waypoints=%s&key=%s", locations[0][0], locations[0][1], perm[permEnd][0], perm[permEnd][1], waypoints, apiKey))
		}
	}

	return urls
}

func CallGMapApi(client *http.Client, urls []string, start [2]string) (types.SuccessResponse, error) {
	var success types.SuccessResponse
	var m types.Message
	var shortestDistance = int64(math.MaxInt64)
	var shortestTime int64
	var shortestPath [][2]string

	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			return success, err
		}

		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&m)

		if len(m.Routes) > 0 {
			path := [][2]string{start}
			distance := int64(0)
			time := int64(0)
			for _, leg := range m.Routes[0].Legs {
				distance += leg.Distance.Value
				time += leg.Duration.Value
				path = append(path, [2]string{strconv.FormatFloat(leg.EndLocation.Lat, 'f', -1, 64), strconv.FormatFloat(leg.EndLocation.Lng, 'f', -1, 64)})
			}

			if distance < shortestDistance {
				shortestDistance = distance
				shortestTime = time
				shortestPath = path
			}
		}
	}

	if len(shortestPath) == 0 {
		return success, errors.New("No Route Found")
	}

	success = types.SuccessResponse{"success", shortestPath, shortestDistance, shortestTime}
	return success, nil
}

func GeneratePermutations(data [][2]string) <-chan [][2]string {
	c := make(chan [][2]string)
	go func(c chan [][2]string) {
		defer close(c)
		permutate(c, data)
	}(c)
	return c
}

func permutate(c chan [][2]string, inputs [][2]string) {
	output := make([][2]string, len(inputs))
	copy(output, inputs)
	c <- output

	size := len(inputs)
	p := make([]int, size+1)
	for i := 0; i < size+1; i++ {
		p[i] = i
	}
	for i := 1; i < size; {
		p[i]--
		j := 0
		if i%2 == 1 {
			j = p[i]
		}
		tmp := inputs[j]
		inputs[j] = inputs[i]
		inputs[i] = tmp
		output := make([][2]string, len(inputs))
		copy(output, inputs)
		c <- output
		for i = 1; p[i] == 0; i++ {
			p[i] = i
		}
	}
}
