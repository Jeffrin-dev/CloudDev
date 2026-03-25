package route53

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const xmlContentType = "text/xml"

type HostedZone struct {
	Id     string
	Name   string
	Config HostedZoneConfig
}

type HostedZoneConfig struct {
	Comment string
}

type ResourceRecordSet struct {
	Name    string
	Type    string
	TTL     int
	Records []string
}

type zoneData struct {
	Zone   HostedZone
	RRsets map[string]ResourceRecordSet
}

type server struct {
	mu         sync.RWMutex
	nextZoneID int
	zones      map[string]*zoneData
}

func newServer() *server {
	return &server{zones: make(map[string]*zoneData)}
}

func Start(port int) error {
	srv := newServer()
	return http.ListenAndServe(fmt.Sprintf(":%d", port), srv)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 2 || parts[0] != "2013-04-01" || parts[1] != "hostedzone" {
		writeError(w, http.StatusNotFound, "NotFound", "Not Found")
		return
	}

	switch {
	case len(parts) == 2 && r.Method == http.MethodPost:
		s.createHostedZone(w, r)
	case len(parts) == 2 && r.Method == http.MethodGet:
		s.listHostedZones(w)
	case len(parts) == 3 && r.Method == http.MethodGet:
		s.getHostedZone(w, parts[2])
	case len(parts) == 3 && r.Method == http.MethodDelete:
		s.deleteHostedZone(w, parts[2])
	case len(parts) == 4 && parts[3] == "rrset" && r.Method == http.MethodPost:
		s.changeResourceRecordSets(w, r, parts[2])
	case len(parts) == 4 && parts[3] == "rrset" && r.Method == http.MethodGet:
		s.listResourceRecordSets(w, parts[2])
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method Not Allowed")
	}
}

type createHostedZoneRequest struct {
	XMLName xml.Name `xml:"CreateHostedZoneRequest"`
	Name    string   `xml:"Name"`
	Comment string   `xml:"HostedZoneConfig>Comment"`
}

func (s *server) createHostedZone(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req createHostedZoneRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidInput", "Invalid XML body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "InvalidInput", "Name is required")
		return
	}

	s.mu.Lock()
	s.nextZoneID++
	zoneID := fmt.Sprintf("Z%d", s.nextZoneID)
	zone := HostedZone{
		Id:   "/hostedzone/" + zoneID,
		Name: name,
		Config: HostedZoneConfig{
			Comment: strings.TrimSpace(req.Comment),
		},
	}
	s.zones[zoneID] = &zoneData{Zone: zone, RRsets: make(map[string]ResourceRecordSet)}
	s.mu.Unlock()

	resp := struct {
		XMLName    xml.Name `xml:"CreateHostedZoneResponse"`
		HostedZone struct {
			Id     string `xml:"Id"`
			Name   string `xml:"Name"`
			Config struct {
				Comment string `xml:"Comment,omitempty"`
			} `xml:"Config"`
		} `xml:"HostedZone"`
	}{}
	resp.HostedZone.Id = zone.Id
	resp.HostedZone.Name = zone.Name
	resp.HostedZone.Config.Comment = zone.Config.Comment
	writeXML(w, http.StatusCreated, resp)
}

func (s *server) listHostedZones(w http.ResponseWriter) {
	s.mu.RLock()
	zones := make([]HostedZone, 0, len(s.zones))
	for _, z := range s.zones {
		zones = append(zones, z.Zone)
	}
	s.mu.RUnlock()

	resp := struct {
		XMLName     xml.Name `xml:"ListHostedZonesResponse"`
		HostedZones []struct {
			Id   string `xml:"Id"`
			Name string `xml:"Name"`
		} `xml:"HostedZones>HostedZone"`
	}{}
	for _, z := range zones {
		resp.HostedZones = append(resp.HostedZones, struct {
			Id   string `xml:"Id"`
			Name string `xml:"Name"`
		}{Id: z.Id, Name: z.Name})
	}
	writeXML(w, http.StatusOK, resp)
}

func (s *server) getHostedZone(w http.ResponseWriter, zoneID string) {
	s.mu.RLock()
	z, ok := s.zones[zoneID]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchHostedZone", "Hosted zone not found")
		return
	}

	resp := struct {
		XMLName    xml.Name `xml:"GetHostedZoneResponse"`
		HostedZone struct {
			Id     string `xml:"Id"`
			Name   string `xml:"Name"`
			Config struct {
				Comment string `xml:"Comment,omitempty"`
			} `xml:"Config"`
		} `xml:"HostedZone"`
	}{}
	resp.HostedZone.Id = z.Zone.Id
	resp.HostedZone.Name = z.Zone.Name
	resp.HostedZone.Config.Comment = z.Zone.Config.Comment
	writeXML(w, http.StatusOK, resp)
}

func (s *server) deleteHostedZone(w http.ResponseWriter, zoneID string) {
	s.mu.Lock()
	_, ok := s.zones[zoneID]
	if ok {
		delete(s.zones, zoneID)
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchHostedZone", "Hosted zone not found")
		return
	}

	resp := struct {
		XMLName string `xml:"DeleteHostedZoneResponse"`
		Status  string `xml:"Status"`
	}{Status: "DELETED"}
	writeXML(w, http.StatusOK, resp)
}

type changeResourceRecordSetsRequest struct {
	XMLName     xml.Name `xml:"ChangeResourceRecordSetsRequest"`
	ChangeBatch struct {
		Changes []struct {
			Action            string `xml:"Action"`
			ResourceRecordSet struct {
				Name            string `xml:"Name"`
				Type            string `xml:"Type"`
				TTL             int    `xml:"TTL"`
				ResourceRecords []struct {
					Value string `xml:"Value"`
				} `xml:"ResourceRecords>ResourceRecord"`
			} `xml:"ResourceRecordSet"`
		} `xml:"Changes>Change"`
	} `xml:"ChangeBatch"`
}

func rrsetKey(name, recordType string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + strings.ToUpper(strings.TrimSpace(recordType))
}

func (s *server) changeResourceRecordSets(w http.ResponseWriter, r *http.Request, zoneID string) {
	defer r.Body.Close()
	var req changeResourceRecordSetsRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidInput", "Invalid XML body")
		return
	}

	s.mu.Lock()
	zone, ok := s.zones[zoneID]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "NoSuchHostedZone", "Hosted zone not found")
		return
	}

	for _, ch := range req.ChangeBatch.Changes {
		action := strings.ToUpper(strings.TrimSpace(ch.Action))
		rrName := strings.TrimSpace(ch.ResourceRecordSet.Name)
		rrType := strings.ToUpper(strings.TrimSpace(ch.ResourceRecordSet.Type))
		if rrName == "" || rrType == "" {
			s.mu.Unlock()
			writeError(w, http.StatusBadRequest, "InvalidInput", "Record name and type are required")
			return
		}

		key := rrsetKey(rrName, rrType)
		records := make([]string, 0, len(ch.ResourceRecordSet.ResourceRecords))
		for _, rec := range ch.ResourceRecordSet.ResourceRecords {
			records = append(records, strings.TrimSpace(rec.Value))
		}
		rrset := ResourceRecordSet{
			Name:    rrName,
			Type:    rrType,
			TTL:     ch.ResourceRecordSet.TTL,
			Records: records,
		}

		switch action {
		case "CREATE":
			if _, exists := zone.RRsets[key]; exists {
				s.mu.Unlock()
				writeError(w, http.StatusBadRequest, "InvalidChangeBatch", "RRSet already exists")
				return
			}
			zone.RRsets[key] = rrset
		case "UPSERT":
			zone.RRsets[key] = rrset
		case "DELETE":
			delete(zone.RRsets, key)
		default:
			s.mu.Unlock()
			writeError(w, http.StatusBadRequest, "InvalidInput", "Unsupported action: "+action)
			return
		}
	}
	s.mu.Unlock()

	resp := struct {
		XMLName string `xml:"ChangeResourceRecordSetsResponse"`
		Status  string `xml:"Status"`
	}{Status: "INSYNC"}
	writeXML(w, http.StatusOK, resp)
}

func (s *server) listResourceRecordSets(w http.ResponseWriter, zoneID string) {
	s.mu.RLock()
	z, ok := s.zones[zoneID]
	if !ok {
		s.mu.RUnlock()
		writeError(w, http.StatusNotFound, "NoSuchHostedZone", "Hosted zone not found")
		return
	}
	items := make([]ResourceRecordSet, 0, len(z.RRsets))
	for _, item := range z.RRsets {
		items = append(items, item)
	}
	s.mu.RUnlock()

	resp := struct {
		XMLName            xml.Name `xml:"ListResourceRecordSetsResponse"`
		ResourceRecordSets []struct {
			Name            string `xml:"Name"`
			Type            string `xml:"Type"`
			TTL             int    `xml:"TTL,omitempty"`
			ResourceRecords []struct {
				Value string `xml:"Value"`
			} `xml:"ResourceRecords>ResourceRecord"`
		} `xml:"ResourceRecordSets>ResourceRecordSet"`
	}{}

	for _, rrset := range items {
		entry := struct {
			Name            string `xml:"Name"`
			Type            string `xml:"Type"`
			TTL             int    `xml:"TTL,omitempty"`
			ResourceRecords []struct {
				Value string `xml:"Value"`
			} `xml:"ResourceRecords>ResourceRecord"`
		}{
			Name: rrset.Name,
			Type: rrset.Type,
			TTL:  rrset.TTL,
		}
		for _, v := range rrset.Records {
			entry.ResourceRecords = append(entry.ResourceRecords, struct {
				Value string `xml:"Value"`
			}{Value: v})
		}
		resp.ResourceRecordSets = append(resp.ResourceRecordSets, entry)
	}

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
