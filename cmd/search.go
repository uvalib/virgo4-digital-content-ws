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
		err := fmt.Errorf("record not found")
		s.err(err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	// verify indexed part field lengths are equal, and all required fields are present

	doc := s.solrRes.Response.Docs[0]

	length := -1
	invalid := false

	for _, field := range s.svc.config.Fields.Parts.Indexed {
		fieldValues := doc.getValuesByTag(field.Field)
		fieldLength := len(fieldValues)

		if field.Required == true && fieldLength == 0 {
			err := fmt.Errorf("missing required digital content field: %s", field.Field)
			s.err(err.Error())
			invalid = true
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
		err := fmt.Errorf("no digital parts found in this record")
		s.err(err.Error())
		return searchResponse{status: http.StatusInternalServerError, err: err}
	}

	// build response object

	var parts []map[string]interface{}

	// assign part-level fields

	for i := 0; i < length; i++ {
		part := make(map[string]interface{})

		for _, field := range s.svc.config.Fields.Parts.Indexed {
			fieldValues := doc.getValuesByTag(field.Field)
			if val := fieldValues[i]; val != "" {
				part[field.Name] = val
			}
		}

		for _, field := range s.svc.config.Fields.Parts.Custom {
			var val interface{}

			fieldValues := doc.getValuesByTag(field.Field)

			switch field.Name {
			case "iiif_manifest_url":
				pid := part["pid"].(string)
				val = fmt.Sprintf("%s/%s", field.CustomInfo.IIIFManifestURL.URLPrefix, pid)

			case "pdf":
				pdfURL := firstElementOf(fieldValues)
				if pdfURL == "" {
					s.log("no pdf url; skipping pdf section")
					continue
				}

				pid := part["pid"].(string)
				if pid == "" {
					s.log("no pid; skipping pdf section")
					continue
				}

				// build a pdf subsection

				pdf := make(map[string]interface{})

				pdfStatus, pdfErr := s.getPdfStatus(pdfURL, pid)
				if pdfErr != nil {
					pdfStatus = ""
				}

				urls := make(map[string]interface{})
				urls["generate"] = fmt.Sprintf("%s/%s%s", pdfURL, pid, s.svc.config.Pdf.Endpoints.Generate)
				urls["status"] = fmt.Sprintf("%s/%s%s", pdfURL, pid, s.svc.config.Pdf.Endpoints.Status)
				urls["download"] = fmt.Sprintf("%s/%s%s", pdfURL, pid, s.svc.config.Pdf.Endpoints.Download)
				urls["delete"] = fmt.Sprintf("%s/%s%s", pdfURL, pid, s.svc.config.Pdf.Endpoints.Delete)

				pdf["status"] = pdfStatus
				pdf["urls"] = urls

				val = pdf
			}

			if val != "" {
				part[field.Name] = val
			}
		}

		parts = append(parts, part)
	}

	item := make(map[string]interface{})

	// assign item-level fields

	for _, field := range s.svc.config.Fields.Item {
		fieldValues := doc.getValuesByTag(field.Field)
		if val := firstElementOf(fieldValues); val != "" {
			item[field.Name] = val
		}
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
