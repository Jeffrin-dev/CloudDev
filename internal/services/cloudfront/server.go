package cloudfront

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const xmlContentType = "application/xml"

type Distribution struct {
	Id         string
	DomainName string
	Status     string
	Origins    []Origin
	Comment    string
}

type Origin struct {
	Id         string
	DomainName string
}

type Invalidation struct {
	Id         string
	Status     string
	CreateTime string
}

type server struct {
	mu                  sync.RWMutex
	nextDistributionID  int
	nextInvalidationID  int
	distributions       map[string]Distribution
	distributionOrder   []string
	invalidationsByDist map[string][]Invalidation
}

func newServer() *server {
	return &server{
		distributions:       make(map[string]Distribution),
		invalidationsByDist: make(map[string][]Invalidation),
	}
}

func Start(port int) error {
	srv := newServer()
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 2 || parts[0] != "2020-05-31" || parts[1] != "distribution" {
		writeError(w, http.StatusNotFound, "NoSuchResource", "Not Found")
		return
	}

	switch {
	case len(parts) == 2 && r.Method == http.MethodPost:
		s.createDistribution(w, r)
	case len(parts) == 2 && r.Method == http.MethodGet:
		s.listDistributions(w)
	case len(parts) == 3 && r.Method == http.MethodGet:
		s.getDistribution(w, parts[2])
	case len(parts) == 3 && r.Method == http.MethodDelete:
		s.deleteDistribution(w, parts[2])
	case len(parts) == 4 && parts[3] == "invalidation" && r.Method == http.MethodPost:
		s.createInvalidation(w, r, parts[2])
	case len(parts) == 4 && parts[3] == "invalidation" && r.Method == http.MethodGet:
		s.listInvalidations(w, parts[2])
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method Not Allowed")
	}
}

type DistributionConfigInput struct {
	Comment string `xml:"Comment"`
	Origins struct {
		Items struct {
			Origins []struct {
				Id         string `xml:"Id"`
				DomainName string `xml:"DomainName"`
			} `xml:"Origin"`
		} `xml:"Items"`
	} `xml:"Origins"`
}

func (s *server) createDistribution(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		DistributionConfig DistributionConfigInput `xml:"DistributionConfig"`
		DistributionConfigInput
	}
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("cloudfront: create distribution decode error: %v", err)
		writeError(w, http.StatusBadRequest, "InvalidArgument", "Invalid XML body")
		return
	}

	input := req.DistributionConfig
	if input.Comment == "" && len(input.Origins.Items.Origins) == 0 {
		input = req.DistributionConfigInput
	}

	origins := make([]Origin, 0, len(input.Origins.Items.Origins))
	for _, item := range input.Origins.Items.Origins {
		origins = append(origins, Origin{
			Id:         strings.TrimSpace(item.Id),
			DomainName: strings.TrimSpace(item.DomainName),
		})
	}

	s.mu.Lock()
	s.nextDistributionID++
	dID := fmt.Sprintf("D%d", s.nextDistributionID)
	dist := Distribution{Id: dID, DomainName: dID + ".cloudfront.localhost", Status: "Deployed", Origins: origins, Comment: strings.TrimSpace(input.Comment)}
	s.distributions[dID] = dist
	s.distributionOrder = append(s.distributionOrder, dID)
	s.mu.Unlock()

	resp := struct {
		XMLName      xml.Name      `xml:"CreateDistributionResponse"`
		Distribution distributionX `xml:"Distribution"`
	}{Distribution: toDistributionX(dist)}
	writeXML(w, http.StatusCreated, resp)
}

type distributionX struct {
	Id         string    `xml:"Id"`
	DomainName string    `xml:"DomainName"`
	Status     string    `xml:"Status"`
	Comment    string    `xml:"Comment,omitempty"`
	Origins    []originX `xml:"Origins>Items>Origin"`
}

type originX struct {
	Id         string `xml:"Id"`
	DomainName string `xml:"DomainName"`
}

func toDistributionX(d Distribution) distributionX {
	o := make([]originX, 0, len(d.Origins))
	for _, item := range d.Origins {
		o = append(o, originX{Id: item.Id, DomainName: item.DomainName})
	}
	return distributionX{Id: d.Id, DomainName: d.DomainName, Status: d.Status, Comment: d.Comment, Origins: o}
}

func (s *server) listDistributions(w http.ResponseWriter) {
	s.mu.RLock()
	items := make([]distributionX, 0, len(s.distributionOrder))
	for _, id := range s.distributionOrder {
		if d, ok := s.distributions[id]; ok {
			items = append(items, toDistributionX(d))
		}
	}
	s.mu.RUnlock()

	resp := struct {
		XMLName       xml.Name        `xml:"ListDistributionsResponse"`
		Distributions []distributionX `xml:"DistributionList>Items>Distribution"`
	}{Distributions: items}
	writeXML(w, http.StatusOK, resp)
}

func (s *server) getDistribution(w http.ResponseWriter, id string) {
	s.mu.RLock()
	d, ok := s.distributions[id]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchDistribution", "Distribution not found")
		return
	}
	resp := struct {
		XMLName      xml.Name      `xml:"GetDistributionResponse"`
		Distribution distributionX `xml:"Distribution"`
	}{Distribution: toDistributionX(d)}
	writeXML(w, http.StatusOK, resp)
}

func (s *server) deleteDistribution(w http.ResponseWriter, id string) {
	s.mu.Lock()
	_, ok := s.distributions[id]
	if ok {
		delete(s.distributions, id)
		delete(s.invalidationsByDist, id)
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchDistribution", "Distribution not found")
		return
	}
	resp := struct {
		XMLName string `xml:"DeleteDistributionResponse"`
		Status  string `xml:"Status"`
	}{Status: "Deleted"}
	writeXML(w, http.StatusOK, resp)
}

type createInvalidationRequest struct {
	XMLName           xml.Name `xml:"CreateInvalidationRequest"`
	InvalidationBatch struct {
		Paths struct {
			Items []string `xml:"Items>Path"`
		} `xml:"Paths"`
	} `xml:"InvalidationBatch"`
}

func (s *server) createInvalidation(w http.ResponseWriter, r *http.Request, distributionID string) {
	defer r.Body.Close()
	_, _ = io.ReadAll(r.Body)

	s.mu.Lock()
	_, ok := s.distributions[distributionID]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "NoSuchDistribution", "Distribution not found")
		return
	}
	s.nextInvalidationID++
	inv := Invalidation{Id: fmt.Sprintf("I%d", s.nextInvalidationID), Status: "Completed", CreateTime: time.Now().UTC().Format(time.RFC3339)}
	s.invalidationsByDist[distributionID] = append(s.invalidationsByDist[distributionID], inv)
	s.mu.Unlock()

	resp := struct {
		XMLName      xml.Name      `xml:"CreateInvalidationResponse"`
		Invalidation invalidationX `xml:"Invalidation"`
	}{Invalidation: toInvalidationX(inv)}
	writeXML(w, http.StatusCreated, resp)
}

type invalidationX struct {
	Id         string `xml:"Id"`
	Status     string `xml:"Status"`
	CreateTime string `xml:"CreateTime"`
}

func toInvalidationX(i Invalidation) invalidationX {
	return invalidationX{Id: i.Id, Status: i.Status, CreateTime: i.CreateTime}
}

func (s *server) listInvalidations(w http.ResponseWriter, distributionID string) {
	s.mu.RLock()
	_, ok := s.distributions[distributionID]
	if !ok {
		s.mu.RUnlock()
		writeError(w, http.StatusNotFound, "NoSuchDistribution", "Distribution not found")
		return
	}
	items := s.invalidationsByDist[distributionID]
	respItems := make([]invalidationX, 0, len(items))
	for _, inv := range items {
		respItems = append(respItems, toInvalidationX(inv))
	}
	s.mu.RUnlock()

	resp := struct {
		XMLName       xml.Name        `xml:"ListInvalidationsResponse"`
		Invalidations []invalidationX `xml:"InvalidationList>Items>Invalidation"`
	}{Invalidations: respItems}
	writeXML(w, http.StatusOK, resp)
}

func writeXML(w http.ResponseWriter, status int, payload any) {
	body, err := xml.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to marshal XML", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header + string(body)))
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	resp := struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Error"`
		StatusCode string `xml:"StatusCode"`
	}{StatusCode: strconv.Itoa(status)}
	resp.Error.Code = code
	resp.Error.Message = message
	writeXML(w, status, resp)
}

func splitPath(path string) []string {
	clean := strings.Trim(path, "/")
	if clean == "" {
		return nil
	}
	return strings.Split(clean, "/")
}
