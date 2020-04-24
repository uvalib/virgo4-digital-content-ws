package main

import (
	"fmt"
	"net/http"
)

type searchContext struct {
	svc     *serviceContext
	client  *clientContext
	query   string
	solrReq *solrRequest
	solrRes *solrResponse
}

type searchResponse struct {
	status int         // http status code
	data   interface{} // data to return as JSON
	err    error       // error, if any
}

func (s *searchContext) init(p *serviceContext, c *clientContext) {
	s.svc = p
	s.client = c
}

func (s *searchContext) log(format string, args ...interface{}) {
	s.client.log(format, args...)
}

func (s *searchContext) err(format string, args ...interface{}) {
	s.client.err(format, args...)
}

func (s *searchContext) handleItemRequest() searchResponse {
	if err := s.solrQuery(); err != nil {
		s.err("query execution error: %s", err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	if s.solrRes.meta.numRows == 0 {
		err := fmt.Errorf("item not found")
		s.err(err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	// verify part field lengths

	doc := s.solrRes.Response.Docs[0]

	length := -1

	for _, field := range s.svc.config.PartFields {
		fieldValues := doc.getValuesByTag(field.Field)
		fieldLength := len(fieldValues)

		s.log("len(%s) = %d", field.Field, fieldLength)

		if length == -1 {
			length = fieldLength
			continue
		}

		if fieldLength != length {
			err := fmt.Errorf("parts field length mismatch")
			s.err(err.Error())
			return searchResponse{status: http.StatusInternalServerError, err: err}
		}
	}

	// build response object
	// might wanna put these in structs for ordering/omitting empty

	item := make(map[string]interface{})

	for _, field := range s.svc.config.ItemFields {
		fieldValues := doc.getValuesByTag(field.Field)
		item[field.Name] = firstElementOf(fieldValues)
	}

	var parts []map[string]interface{}

	for i := 0; i < length; i++ {
		part := make(map[string]interface{})

		for _, field := range s.svc.config.PartFields {
			fieldValues := doc.getValuesByTag(field.Field)
			part[field.Name] = fieldValues[i]
		}

		parts = append(parts, part)
	}

	item["parts"] = parts

	return searchResponse{status: http.StatusOK, data: item}
}

func (s *searchContext) handlePingRequest() searchResponse {
	if err := s.solrQuery(); err != nil {
		s.err("query execution error: %s", err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	return searchResponse{status: http.StatusOK}
}
