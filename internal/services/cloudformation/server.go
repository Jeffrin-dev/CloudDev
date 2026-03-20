package cloudformation

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const xmlContentType = "text/xml"

type Stack struct {
	StackName   string
	StackId     string
	StackStatus string
	Template    string
	Resources   []StackResource
}

type StackResource struct {
	LogicalResourceId  string
	PhysicalResourceId string
	ResourceType       string
	Service            string
	ResourceStatus     string
}

type server struct {
	mu          sync.Mutex
	port        int
	stacks      map[string]Stack
	nextStackID int
}

func newServer(port int) *server {
	return &server{port: port, stacks: make(map[string]Stack)}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer(port))
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "InvalidAction", "Only POST is supported")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, "InvalidParameterValue", "Could not parse form body")
		return
	}

	switch r.FormValue("Action") {
	case "CreateStack":
		s.createStack(w, r)
	case "UpdateStack":
		s.updateStack(w, r)
	case "DeleteStack":
		s.deleteStack(w, r)
	case "ListStacks":
		s.listStacks(w)
	case "DescribeStacks":
		s.describeStacks(w, r)
	case "DescribeStackResources":
		s.describeStackResources(w, r)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) createStack(w http.ResponseWriter, r *http.Request) {
	stackName := r.FormValue("StackName")
	templateBody := r.FormValue("TemplateBody")
	if stackName == "" || templateBody == "" {
		writeError(w, "MissingParameter", "StackName and TemplateBody are required")
		return
	}

	resources, err := parseTemplateResources(templateBody)
	if err != nil {
		writeError(w, "ValidationError", err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextStackID++
	stack := Stack{
		StackName:   stackName,
		StackId:     fmt.Sprintf("arn:aws:cloudformation:us-east-1:000000000000:stack/%s/%d", stackName, s.nextStackID),
		StackStatus: "CREATE_COMPLETE",
		Template:    templateBody,
		Resources:   resources,
	}
	s.stacks[stackName] = stack

	writeXML(w, fmt.Sprintf("<CreateStackResponse><CreateStackResult><StackId>%s</StackId></CreateStackResult><ResponseMetadata><RequestId>req-createstack</RequestId></ResponseMetadata></CreateStackResponse>", xmlEscape(stack.StackId)))
}

func (s *server) updateStack(w http.ResponseWriter, r *http.Request) {
	stackName := r.FormValue("StackName")
	templateBody := r.FormValue("TemplateBody")
	if stackName == "" || templateBody == "" {
		writeError(w, "MissingParameter", "StackName and TemplateBody are required")
		return
	}

	resources, err := parseTemplateResources(templateBody)
	if err != nil {
		writeError(w, "ValidationError", err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	stack, ok := s.stacks[stackName]
	if !ok {
		writeError(w, "ValidationError", "Stack does not exist")
		return
	}
	stack.Template = templateBody
	stack.Resources = resources
	stack.StackStatus = "UPDATE_COMPLETE"
	s.stacks[stackName] = stack

	writeXML(w, fmt.Sprintf("<UpdateStackResponse><UpdateStackResult><StackId>%s</StackId></UpdateStackResult><ResponseMetadata><RequestId>req-updatestack</RequestId></ResponseMetadata></UpdateStackResponse>", xmlEscape(stack.StackId)))
}

func (s *server) deleteStack(w http.ResponseWriter, r *http.Request) {
	stackName := r.FormValue("StackName")
	if stackName == "" {
		writeError(w, "MissingParameter", "StackName is required")
		return
	}

	s.mu.Lock()
	stack, ok := s.stacks[stackName]
	if !ok {
		s.mu.Unlock()
		writeError(w, "ValidationError", "Stack does not exist")
		return
	}
	delete(s.stacks, stackName)
	s.mu.Unlock()

	writeXML(w, fmt.Sprintf("<DeleteStackResponse><DeleteStackResult><StackId>%s</StackId></DeleteStackResult><ResponseMetadata><RequestId>req-deletestack</RequestId></ResponseMetadata></DeleteStackResponse>", xmlEscape(stack.StackId)))
}

func (s *server) listStacks(w http.ResponseWriter) {
	s.mu.Lock()
	stacks := make([]Stack, 0, len(s.stacks))
	for _, stack := range s.stacks {
		stacks = append(stacks, stack)
	}
	s.mu.Unlock()

	sort.Slice(stacks, func(i, j int) bool { return stacks[i].StackName < stacks[j].StackName })
	members := ""
	for _, stack := range stacks {
		members += fmt.Sprintf("<member><StackId>%s</StackId><StackName>%s</StackName><StackStatus>%s</StackStatus></member>", xmlEscape(stack.StackId), xmlEscape(stack.StackName), xmlEscape(stack.StackStatus))
	}

	writeXML(w, fmt.Sprintf("<ListStacksResponse><ListStacksResult><StackSummaries>%s</StackSummaries></ListStacksResult><ResponseMetadata><RequestId>req-liststacks</RequestId></ResponseMetadata></ListStacksResponse>", members))
}

func (s *server) describeStacks(w http.ResponseWriter, r *http.Request) {
	stack, ok := s.findStack(r.FormValue("StackName"))
	if !ok {
		writeError(w, "ValidationError", "Stack does not exist")
		return
	}

	stackXML := fmt.Sprintf("<member><StackId>%s</StackId><StackName>%s</StackName><StackStatus>%s</StackStatus><TemplateBody>%s</TemplateBody></member>", xmlEscape(stack.StackId), xmlEscape(stack.StackName), xmlEscape(stack.StackStatus), xmlEscape(stack.Template))
	writeXML(w, fmt.Sprintf("<DescribeStacksResponse><DescribeStacksResult><Stacks>%s</Stacks></DescribeStacksResult><ResponseMetadata><RequestId>req-describestacks</RequestId></ResponseMetadata></DescribeStacksResponse>", stackXML))
}

func (s *server) describeStackResources(w http.ResponseWriter, r *http.Request) {
	stack, ok := s.findStack(r.FormValue("StackName"))
	if !ok {
		writeError(w, "ValidationError", "Stack does not exist")
		return
	}

	members := ""
	for _, resource := range stack.Resources {
		members += fmt.Sprintf("<member><StackName>%s</StackName><StackId>%s</StackId><LogicalResourceId>%s</LogicalResourceId><PhysicalResourceId>%s</PhysicalResourceId><ResourceType>%s</ResourceType><ResourceStatus>%s</ResourceStatus><Service>%s</Service></member>",
			xmlEscape(stack.StackName),
			xmlEscape(stack.StackId),
			xmlEscape(resource.LogicalResourceId),
			xmlEscape(resource.PhysicalResourceId),
			xmlEscape(resource.ResourceType),
			xmlEscape(resource.ResourceStatus),
			xmlEscape(resource.Service),
		)
	}

	writeXML(w, fmt.Sprintf("<DescribeStackResourcesResponse><DescribeStackResourcesResult><StackResources>%s</StackResources></DescribeStackResourcesResult><ResponseMetadata><RequestId>req-describestackresources</RequestId></ResponseMetadata></DescribeStackResourcesResponse>", members))
}

func (s *server) findStack(stackName string) (Stack, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stackName != "" {
		stack, ok := s.stacks[stackName]
		return stack, ok
	}
	if len(s.stacks) == 1 {
		for _, stack := range s.stacks {
			return stack, true
		}
	}
	return Stack{}, false
}

func parseTemplateResources(templateBody string) ([]StackResource, error) {
	var root map[string]any
	trimmed := strings.TrimSpace(templateBody)
	if trimmed == "" {
		return nil, fmt.Errorf("template body is empty")
	}

	var err error
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		err = json.Unmarshal([]byte(templateBody), &root)
	} else {
		err = yaml.Unmarshal([]byte(templateBody), &root)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid template body: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("invalid template body")
	}

	resourcesRaw, ok := root["Resources"]
	if !ok {
		return nil, nil
	}
	resourcesMap, ok := resourcesRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("template Resources must be an object")
	}

	logicalIDs := make([]string, 0, len(resourcesMap))
	for logicalID := range resourcesMap {
		logicalIDs = append(logicalIDs, logicalID)
	}
	sort.Strings(logicalIDs)

	resources := make([]StackResource, 0, len(resourcesMap))
	for _, logicalID := range logicalIDs {
		resourceDef, ok := resourcesMap[logicalID].(map[string]any)
		if !ok {
			continue
		}
		resourceType, _ := resourceDef["Type"].(string)
		resources = append(resources, StackResource{
			LogicalResourceId:  logicalID,
			PhysicalResourceId: logicalID,
			ResourceType:       resourceType,
			Service:            mapService(resourceType),
			ResourceStatus:     "CREATE_COMPLETE",
		})
	}

	return resources, nil
}

func mapService(resourceType string) string {
	switch resourceType {
	case "AWS::S3::Bucket":
		return "s3"
	case "AWS::DynamoDB::Table":
		return "dynamodb"
	case "AWS::Lambda::Function":
		return "lambda"
	case "AWS::SQS::Queue":
		return "sqs"
	case "AWS::ApiGateway::RestApi":
		return "api_gateway"
	case "AWS::IAM::Role":
		return "iam"
	case "AWS::SecretsManager::Secret":
		return "secretsmanager"
	case "AWS::Logs::LogGroup":
		return "cloudwatchlogs"
	case "AWS::KMS::Key":
		return "kms"
	default:
		return "unknown"
	}
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<?xml version=\"1.0\"?>" + body))
}

func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>", xmlEscape(code), xmlEscape(message)))
}

func xmlEscape(value string) string {
	var builder strings.Builder
	if err := xml.EscapeText(&builder, []byte(value)); err != nil {
		return value
	}
	return builder.String()
}
