package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type solrRequestParams struct {
	DefType string   `json:"defType,omitempty"`
	Qt      string   `json:"qt,omitempty"`
	Sort    string   `json:"sort,omitempty"`
	Start   int      `json:"start"`
	Rows    int      `json:"rows"`
	Fl      []string `json:"fl,omitempty"`
	Fq      []string `json:"fq,omitempty"`
	Q       string   `json:"q,omitempty"`
}

type solrRequestJSON struct {
	Params solrRequestParams `json:"params"`
}

type solrMeta struct {
	maxScore  float32
	start     int
	numRows   int // for client pagination -- numGroups or numRecords
	totalRows int // for client pagination -- totalGroups or totalRecords
}

type solrRequest struct {
	json solrRequestJSON
	meta solrMeta
}

type solrResponseHeader struct {
	Status int `json:"status,omitempty"`
	QTime  int `json:"QTime,omitempty"`
}

type solrDocument struct {
	AlternateID          []string `json:"alternate_id_a,omitempty"`
	ID                   string   `json:"id,omitempty"`
	IndividualCallNumber []string `json:"individual_call_number_a,omitempty"`
	PDFURL               []string `json:"pdf_url_a,omitempty"`
	ThumbnailURL         []string `json:"thumbnail_url_a,omitempty"`
	URLIIIFManifest      string   `json:"url_iiif_manifest_stored,omitempty"`
	RightsWrapperURL     []string `json:"rights_wrapper_url_a,omitempty"`
	Score                float32  `json:"score,omitempty"`
}

type solrResponseDocuments struct {
	NumFound int            `json:"numFound,omitempty"`
	Start    int            `json:"start,omitempty"`
	MaxScore float32        `json:"maxScore,omitempty"`
	Docs     []solrDocument `json:"docs,omitempty"`
}

type solrError struct {
	Metadata []string `json:"metadata,omitempty"`
	Msg      string   `json:"msg,omitempty"`
	Code     int      `json:"code,omitempty"`
}

type solrResponse struct {
	ResponseHeader solrResponseHeader    `json:"responseHeader,omitempty"`
	Response       solrResponseDocuments `json:"response,omitempty"`
	Error          solrError             `json:"error,omitempty"`
	meta           *solrMeta             // pointer to struct in corresponding solrRequest
}

func (s *solrDocument) getFieldByTag(tag string) interface{} {
	rt := reflect.TypeOf(*s)

	if rt.Kind() != reflect.Struct {
		return nil
	}

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		v := strings.Split(f.Tag.Get("json"), ",")[0]
		if v == tag {
			return reflect.ValueOf(*s).Field(i).Interface()
		}
	}

	return nil
}

func (s *solrDocument) getValuesByTag(tag string) []string {
	// turn all potential values into string slices

	v := s.getFieldByTag(tag)

	switch t := v.(type) {
	case []string:
		return t

	case string:
		return []string{t}

	case float32:
		// in case this is ever called for fields such as 'score'
		return []string{fmt.Sprintf("%0.8f", t)}

	default:
		return []string{}
	}
}

func (s *searchContext) buildSolrRequest() {
	var req solrRequest

	//	req.meta.client = s.virgoReq.meta.client

	req.json.Params.Q = s.query
	req.json.Params.Qt = s.svc.config.Solr.Params.Qt
	req.json.Params.DefType = s.svc.config.Solr.Params.DefType
	req.json.Params.Fq = nonemptyValues(s.svc.config.Solr.Params.Fq)
	req.json.Params.Fl = nonemptyValues(s.svc.config.Solr.Params.Fl)
	req.json.Params.Start = 0
	req.json.Params.Rows = 1

	s.solrReq = &req
}

func (s *searchContext) solrQuery() error {
	s.buildSolrRequest()

	jsonBytes, jsonErr := json.Marshal(s.solrReq.json)
	if jsonErr != nil {
		s.log("[SOLR] Marshal() failed: %s", jsonErr.Error())
		return fmt.Errorf("failed to marshal Solr JSON")
	}

	// we cannot use query parameters for the request due to the
	// possibility of triggering a 414 response (URI Too Long).

	// instead, write the json to the body of the request.
	// NOTE: Solr is lenient; GET or POST works fine for this.

	req, reqErr := http.NewRequest("POST", s.svc.solr.url, bytes.NewBuffer(jsonBytes))
	if reqErr != nil {
		s.log("[SOLR] NewRequest() failed: %s", reqErr.Error())
		return fmt.Errorf("failed to create Solr request")
	}

	req.Header.Set("Content-Type", "application/json")

	if s.client.opts.verbose == true {
		s.log("[SOLR] req: [%s]", string(jsonBytes))
	} else {
		s.log("[SOLR] req: [%s]", s.solrReq.json.Params.Q)
	}

	start := time.Now()
	res, resErr := s.svc.solr.client.Do(req)
	elapsedMS := int64(time.Since(start) / time.Millisecond)

	// external service failure logging (scenario 1)

	if resErr != nil {
		status := http.StatusBadRequest
		errMsg := resErr.Error()
		if strings.Contains(errMsg, "Timeout") {
			status = http.StatusRequestTimeout
			errMsg = fmt.Sprintf("%s timed out", s.svc.solr.url)
		} else if strings.Contains(errMsg, "connection refused") {
			status = http.StatusServiceUnavailable
			errMsg = fmt.Sprintf("%s refused connection", s.svc.solr.url)
		}

		s.log("[SOLR] client.Do() failed: %s", resErr.Error())
		s.log("ERROR: Failed response from %s %s - %d:%s. Elapsed Time: %d (ms)", req.Method, s.svc.solr.url, status, errMsg, elapsedMS)
		return fmt.Errorf("failed to receive Solr response")
	}

	defer res.Body.Close()

	var solrRes solrResponse

	decoder := json.NewDecoder(res.Body)

	// external service failure logging (scenario 2)

	if decErr := decoder.Decode(&solrRes); decErr != nil {
		s.log("[SOLR] Decode() failed: %s", decErr.Error())
		s.log("ERROR: Failed response from %s %s - %d:%s. Elapsed Time: %d (ms)", req.Method, s.svc.solr.url, http.StatusInternalServerError, decErr.Error(), elapsedMS)
		return fmt.Errorf("failed to decode Solr response")
	}

	// external service success logging

	s.log("Successful Solr response from %s %s. Elapsed Time: %d (ms)", req.Method, s.svc.solr.url, elapsedMS)

	s.solrRes = &solrRes

	// log abbreviated results

	logHeader := fmt.Sprintf("[SOLR] res: header: { status = %d, QTime = %d }", solrRes.ResponseHeader.Status, solrRes.ResponseHeader.QTime)

	// quick validation
	if solrRes.ResponseHeader.Status != 0 {
		s.log("%s, error: { code = %d, msg = %s }", logHeader, solrRes.Error.Code, solrRes.Error.Msg)
		return fmt.Errorf("%d - %s", solrRes.Error.Code, solrRes.Error.Msg)
	}

	s.solrRes.meta = &s.solrReq.meta
	s.solrRes.meta.start = s.solrReq.json.Params.Start
	s.solrRes.meta.numRows = len(s.solrRes.Response.Docs)
	s.solrRes.meta.totalRows = s.solrRes.Response.NumFound

	s.log("%s, body: { start = %d, rows = %d, total = %d, maxScore = %0.2f }", logHeader, solrRes.meta.start, solrRes.meta.numRows, solrRes.meta.totalRows, solrRes.meta.maxScore)

	return nil
}
