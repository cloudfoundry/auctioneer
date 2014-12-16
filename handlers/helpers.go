package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func writeInvalidJSONResponse(w http.ResponseWriter, err error) {
	writeJSONResponse(w, http.StatusBadRequest, HandlerError{
		Error: err.Error(),
	})
}

func writeStatusCreatedResponse(w http.ResponseWriter) {
	writeJSONResponse(w, http.StatusCreated, struct{}{})
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, jsonObj interface{}) {
	jsonBytes, err := json.Marshal(jsonObj)
	if err != nil {
		panic("Unable to encode JSON: " + err.Error())
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(jsonBytes)))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	_, err = w.Write(jsonBytes)
	if err != nil {
		panic("Unable to write response: " + err.Error())
	}
}

type HandlerError struct {
	Error string `json:"error"`
}
