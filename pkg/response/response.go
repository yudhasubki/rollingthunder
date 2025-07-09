package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type BaseResponse[T any] struct {
	Errors []BaseErrorResponse `json:"errors,omitempty"`
	Data   T                   `json:"data,omitempty"`
}

type BaseErrorResponse struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

type BaseWithMeta[R, T any] struct {
	BaseResponse[R]
	Items BaseMetaItems[T] `json:"meta,omitempty"`
}

type BaseMetaItems[T any] struct {
	Size  *int64 `json:"size,omitempty"`
	Page  *int64 `json:"page,omitempty"`
	Items *T     `json:"items,omitempty"`
}

func ErrorInternal[R any](errs ...BaseErrorResponse) BaseResponse[R] {
	for _, r := range errs {
		r.Status = http.StatusInternalServerError
	}

	return BaseResponse[R]{
		Errors: errs,
	}
}

func OKWithMeta[R, T any](w http.ResponseWriter, data R, meta BaseMetaItems[T]) {
	Respond(BaseWithMeta[R, T]{
		BaseResponse: BaseResponse[R]{
			Data: data,
		},
		Items: meta,
	}, http.StatusOK, w)
}

func OK[R any](w http.ResponseWriter, data R) {
	Respond(BaseResponse[R]{
		Data: data,
	}, http.StatusOK, w)
}

func InternalServerError[R any](w http.ResponseWriter, errors ...BaseErrorResponse) {
	Respond(BaseResponse[R]{
		Errors: errors,
	}, http.StatusInternalServerError, w)
}

func BadRequest[R any](w http.ResponseWriter, errors ...BaseErrorResponse) {
	Respond(BaseResponse[R]{
		Errors: errors,
	}, http.StatusBadRequest, w)
}

func Unauthorized[R any](w http.ResponseWriter, errors ...BaseErrorResponse) {
	Respond(BaseResponse[R]{
		Errors: errors,
	}, http.StatusUnauthorized, w)
}

func Respond(b interface{}, statusCode int, w http.ResponseWriter) {
	response, _ := json.Marshal(b)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, err := w.Write(response)
	if err != nil {
		slog.Error(err.Error())
	}
}
