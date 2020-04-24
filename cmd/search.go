package main

import (
	"fmt"
	"net/http"
)

type searchContext struct {
	svc     *serviceContext
	client  *clientContext
	id      string
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

	// verify array-type field lengths are equal, and all required fields are presenst

	doc := s.solrRes.Response.Docs[0]

	length := -1

	for _, field := range s.svc.config.Fields {
		fieldValues := doc.getValuesByTag(field.Field)
		fieldLength := len(fieldValues)

		if field.Required == true && fieldLength == 0 {
			err := fmt.Errorf("missing required digital content field(s)")
			s.err(err.Error())
			return searchResponse{status: http.StatusInternalServerError, err: err}
		}

		if field.Array == false {
			continue
		}

		s.log("%d = len(%s)", fieldLength, field.Field)

		if length == -1 {
			length = fieldLength
			continue
		}

		if fieldLength != length {
			err := fmt.Errorf("array-type field length mismatch")
			s.err(err.Error())
			return searchResponse{status: http.StatusInternalServerError, err: err}
		}
	}

	if length == 0 {
		err := fmt.Errorf("no digital parts found in this item")
		s.err(err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	// build response object

	var parts []map[string]interface{}

	for i := 0; i < length; i++ {
		part := make(map[string]interface{})

		for _, field := range s.svc.config.Fields {
			fieldValues := doc.getValuesByTag(field.Field)

			var fieldValue string

			if field.Custom == true {
				switch field.Name {
				case "iiif_manifest_url":
					fieldValue = ""

				case "pdf_status":
					fieldValue = ""

				case "pdf_url":
					fieldValue = ""

				default:
				}
			} else {
				if field.Array == true {
					fieldValue = fieldValues[i]
				} else {
					// what should this be?
					fieldValue = firstElementOf(fieldValues)
				}
			}

			if fieldValue != "" {
				part[field.Name] = fieldValue
			}
		}

		parts = append(parts, part)
	}

	item := make(map[string]interface{})

	item["id"] = s.id
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
