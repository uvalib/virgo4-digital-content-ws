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

func (s *searchContext) handleResourceRequest() searchResponse {
	if err := s.solrQuery(); err != nil {
		s.err("query execution error: %s", err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	if s.solrRes.meta.numRows == 0 {
		err := fmt.Errorf("record not found")
		s.err(err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	// verify array-type field lengths are equal, and all required fields are presenst

	doc := s.solrRes.Response.Docs[0]

	length := -1
	invalid := false

	for _, field := range s.svc.config.Fields {
		fieldValues := doc.getValuesByTag(field.Field)
		fieldLength := len(fieldValues)

		if field.Required == true && fieldLength == 0 {
			err := fmt.Errorf("missing required digital content field: %s", field.Field)
			s.err(err.Error())
			invalid = true
			continue
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
			err := fmt.Errorf("array-type field length mismatch for field: %s", field.Field)
			s.err(err.Error())
			invalid = true
			continue
		}
	}

	if invalid == true {
		err := fmt.Errorf("digital content field inconsistencies")
		s.err(err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	if length == 0 {
		err := fmt.Errorf("no digital items found in this record")
		s.err(err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	// build response object

	var items []map[string]interface{}

	for i := 0; i < length; i++ {
		item := make(map[string]interface{})

		// first pass: assign non-custom fields (may be needed in second pass)

		for _, field := range s.svc.config.Fields {
			var fieldValue string

			if field.Custom == false {
				fieldValues := doc.getValuesByTag(field.Field)

				if field.Array == true {
					fieldValue = fieldValues[i]
				} else {
					// what should this be?
					fieldValue = firstElementOf(fieldValues)
				}
			}

			if fieldValue != "" {
				item[field.Name] = fieldValue
			}
		}

		// second pass: build custom fields
		for _, field := range s.svc.config.Fields {
			var fieldValue string

			if field.Custom == true {
				fieldValues := doc.getValuesByTag(field.Field)

				switch field.Name {
				case "iiif_manifest_url":
					//fieldValue = firstElementOf(fieldValues)

				case "pdf_status":
					pdfURL := firstElementOf(fieldValues)
					pdfPID := item["pid"].(string)

					pdfStatus, pdfErr := s.getPdfStatus(pdfURL, pdfPID)
					if pdfErr != nil {
						pdfStatus = ""
					}

					fieldValue = pdfStatus
				}
			}

			if fieldValue != "" {
				item[field.Name] = fieldValue
			}
		}

		items = append(items, item)
	}

	record := make(map[string]interface{})

	record["id"] = s.id
	record["items"] = items

	return searchResponse{status: http.StatusOK, data: record}
}

func (s *searchContext) handlePingRequest() searchResponse {
	if err := s.solrQuery(); err != nil {
		s.err("query execution error: %s", err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	return searchResponse{status: http.StatusOK}
}
