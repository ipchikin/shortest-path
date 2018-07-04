package types

type Message struct {
	Routes []Route `json:"routes"`
}

type Route struct {
	Legs []Leg `json:"legs"`
}

type Leg struct {
	Distance Distance `json:"distance"`
	Duration Duration `json:"duration"`
	Steps    []Step   `json:"steps"`
}

type Distance struct {
	Value int64 `json:"value"`
}

type Duration struct {
	Value int64 `json:"value"`
}

type Step struct {
	EndLocation Location `json:"end_location"`
}

type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}
