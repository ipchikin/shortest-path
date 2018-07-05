package types

type ErrorResponse struct {
	Error string `json:"error"`
}

type Response struct {
	Status string `json:"status"`
}

type SuccessResponse struct {
	Status        string      `json:"status"`
	Path          [][2]string `json:"path"`
	TotalDistance int64       `json:"total_distance"`
	TotalTime     int64       `json:"total_time"`
}

type FailureResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type TokenResponse struct {
	Token string `json:"token"`
}
