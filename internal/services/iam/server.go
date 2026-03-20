package iam

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const xmlContentType = "text/xml"

const (
	accountID = "000000000000"
	region    = "us-east-1"
)

type User struct{ UserName, UserId, Arn string }

type Role struct{ RoleName, RoleId, Arn, AssumeRolePolicyDocument string }

type Policy struct{ PolicyName, PolicyArn, Document string }

type server struct {
	mu           sync.Mutex
	users        map[string]User
	roles        map[string]Role
	policies     map[string]Policy
	attachments  map[string]map[string]bool
	nextUserID   int
	nextRoleID   int
	nextPolicyID int
}

func newServer() *server {
	return &server{
		users:       make(map[string]User),
		roles:       make(map[string]Role),
		policies:    make(map[string]Policy),
		attachments: make(map[string]map[string]bool),
	}
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
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
	case "CreateUser":
		s.createUser(w, r)
	case "DeleteUser":
		s.deleteUser(w, r)
	case "ListUsers":
		s.listUsers(w)
	case "CreateRole":
		s.createRole(w, r)
	case "DeleteRole":
		s.deleteRole(w, r)
	case "ListRoles":
		s.listRoles(w)
	case "CreatePolicy":
		s.createPolicy(w, r)
	case "AttachRolePolicy":
		s.attachRolePolicy(w, r)
	case "DetachRolePolicy":
		s.detachRolePolicy(w, r)
	case "AssumeRole":
		s.assumeRole(w, r)
	default:
		writeError(w, "InvalidAction", "Unknown or missing Action")
	}
}

func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	userName := r.FormValue("UserName")
	if userName == "" {
		writeError(w, "MissingParameter", "UserName is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.users[userName]; ok {
		writeXML(w, fmt.Sprintf("<CreateUserResponse><CreateUserResult><User>%s</User></CreateUserResult><ResponseMetadata><RequestId>req-createuser</RequestId></ResponseMetadata></CreateUserResponse>", userXML(existing)))
		return
	}
	s.nextUserID++
	user := User{UserName: userName, UserId: fmt.Sprintf("AID%012d", s.nextUserID), Arn: userARN(userName)}
	s.users[userName] = user
	writeXML(w, fmt.Sprintf("<CreateUserResponse><CreateUserResult><User>%s</User></CreateUserResult><ResponseMetadata><RequestId>req-createuser</RequestId></ResponseMetadata></CreateUserResponse>", userXML(user)))
}

func (s *server) deleteUser(w http.ResponseWriter, r *http.Request) {
	userName := r.FormValue("UserName")
	if userName == "" {
		writeError(w, "MissingParameter", "UserName is required")
		return
	}

	s.mu.Lock()
	delete(s.users, userName)
	s.mu.Unlock()
	writeXML(w, "<DeleteUserResponse><ResponseMetadata><RequestId>req-deleteuser</RequestId></ResponseMetadata></DeleteUserResponse>")
}

func (s *server) listUsers(w http.ResponseWriter) {
	s.mu.Lock()
	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	s.mu.Unlock()
	sort.Slice(users, func(i, j int) bool { return users[i].UserName < users[j].UserName })

	members := ""
	for _, user := range users {
		members += "<member>" + userXML(user) + "</member>"
	}
	writeXML(w, fmt.Sprintf("<ListUsersResponse><ListUsersResult><Users>%s</Users></ListUsersResult><ResponseMetadata><RequestId>req-listusers</RequestId></ResponseMetadata></ListUsersResponse>", members))
}

func (s *server) createRole(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policyDocument := r.FormValue("AssumeRolePolicyDocument")
	if roleName == "" || policyDocument == "" {
		writeError(w, "MissingParameter", "RoleName and AssumeRolePolicyDocument are required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.roles[roleName]; ok {
		writeXML(w, fmt.Sprintf("<CreateRoleResponse><CreateRoleResult><Role>%s</Role></CreateRoleResult><ResponseMetadata><RequestId>req-createrole</RequestId></ResponseMetadata></CreateRoleResponse>", roleXML(existing)))
		return
	}
	s.nextRoleID++
	role := Role{RoleName: roleName, RoleId: fmt.Sprintf("ARO%012d", s.nextRoleID), Arn: roleARN(roleName), AssumeRolePolicyDocument: policyDocument}
	s.roles[roleName] = role
	writeXML(w, fmt.Sprintf("<CreateRoleResponse><CreateRoleResult><Role>%s</Role></CreateRoleResult><ResponseMetadata><RequestId>req-createrole</RequestId></ResponseMetadata></CreateRoleResponse>", roleXML(role)))
}

func (s *server) deleteRole(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	if roleName == "" {
		writeError(w, "MissingParameter", "RoleName is required")
		return
	}

	s.mu.Lock()
	delete(s.roles, roleName)
	delete(s.attachments, roleName)
	s.mu.Unlock()
	writeXML(w, "<DeleteRoleResponse><ResponseMetadata><RequestId>req-deleterole</RequestId></ResponseMetadata></DeleteRoleResponse>")
}

func (s *server) listRoles(w http.ResponseWriter) {
	s.mu.Lock()
	roles := make([]Role, 0, len(s.roles))
	for _, role := range s.roles {
		roles = append(roles, role)
	}
	s.mu.Unlock()
	sort.Slice(roles, func(i, j int) bool { return roles[i].RoleName < roles[j].RoleName })

	members := ""
	for _, role := range roles {
		members += "<member>" + roleXML(role) + "</member>"
	}
	writeXML(w, fmt.Sprintf("<ListRolesResponse><ListRolesResult><Roles>%s</Roles></ListRolesResult><ResponseMetadata><RequestId>req-listroles</RequestId></ResponseMetadata></ListRolesResponse>", members))
}

func (s *server) createPolicy(w http.ResponseWriter, r *http.Request) {
	policyName := r.FormValue("PolicyName")
	document := r.FormValue("PolicyDocument")
	if policyName == "" || document == "" {
		writeError(w, "MissingParameter", "PolicyName and PolicyDocument are required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, policy := range s.policies {
		if policy.PolicyName == policyName {
			writeXML(w, fmt.Sprintf("<CreatePolicyResponse><CreatePolicyResult><Policy>%s</Policy></CreatePolicyResult><ResponseMetadata><RequestId>req-createpolicy</RequestId></ResponseMetadata></CreatePolicyResponse>", policyXML(policy)))
			return
		}
	}
	s.nextPolicyID++
	arn := policyARN(policyName)
	policy := Policy{PolicyName: policyName, PolicyArn: arn, Document: document}
	s.policies[arn] = policy
	writeXML(w, fmt.Sprintf("<CreatePolicyResponse><CreatePolicyResult><Policy>%s</Policy></CreatePolicyResult><ResponseMetadata><RequestId>req-createpolicy</RequestId></ResponseMetadata></CreatePolicyResponse>", policyXML(policy)))
}

func (s *server) attachRolePolicy(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policyArn := r.FormValue("PolicyArn")
	if roleName == "" || policyArn == "" {
		writeError(w, "MissingParameter", "RoleName and PolicyArn are required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[roleName]; !ok {
		writeError(w, "NoSuchEntity", "Role does not exist")
		return
	}
	if _, ok := s.policies[policyArn]; !ok {
		writeError(w, "NoSuchEntity", "Policy does not exist")
		return
	}
	if s.attachments[roleName] == nil {
		s.attachments[roleName] = make(map[string]bool)
	}
	s.attachments[roleName][policyArn] = true
	writeXML(w, "<AttachRolePolicyResponse><ResponseMetadata><RequestId>req-attachrolepolicy</RequestId></ResponseMetadata></AttachRolePolicyResponse>")
}

func (s *server) detachRolePolicy(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policyArn := r.FormValue("PolicyArn")
	if roleName == "" || policyArn == "" {
		writeError(w, "MissingParameter", "RoleName and PolicyArn are required")
		return
	}

	s.mu.Lock()
	if s.attachments[roleName] != nil {
		delete(s.attachments[roleName], policyArn)
	}
	s.mu.Unlock()
	writeXML(w, "<DetachRolePolicyResponse><ResponseMetadata><RequestId>req-detachrolepolicy</RequestId></ResponseMetadata></DetachRolePolicyResponse>")
}

func (s *server) assumeRole(w http.ResponseWriter, r *http.Request) {
	roleArn := r.FormValue("RoleArn")
	if roleArn == "" {
		writeError(w, "MissingParameter", "RoleArn is required")
		return
	}
	roleName := roleNameFromARN(roleArn)
	if roleName == "" {
		writeError(w, "ValidationError", "RoleArn is invalid")
		return
	}

	s.mu.Lock()
	role, ok := s.roles[roleName]
	s.mu.Unlock()
	if !ok {
		writeError(w, "NoSuchEntity", "Role does not exist")
		return
	}

	expiration := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	writeXML(w, fmt.Sprintf("<AssumeRoleResponse><AssumeRoleResult><AssumedRoleUser><Arn>%s</Arn><AssumedRoleId>%s:%s</AssumedRoleId></AssumedRoleUser><Credentials><AccessKeyId>AKIAIOSFODNN7EXAMPLE</AccessKeyId><SecretAccessKey>wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY</SecretAccessKey><SessionToken>mock-session-token</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleResult><ResponseMetadata><RequestId>req-assumerole</RequestId></ResponseMetadata></AssumeRoleResponse>", role.Arn, role.RoleId, r.FormValue("RoleSessionName"), expiration))
}

func userARN(name string) string   { return fmt.Sprintf("arn:aws:iam::%s:user/%s", accountID, name) }
func roleARN(name string) string   { return fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, name) }
func policyARN(name string) string { return fmt.Sprintf("arn:aws:iam::%s:policy/%s", accountID, name) }

func roleNameFromARN(arn string) string {
	prefix := fmt.Sprintf("arn:aws:iam::%s:role/", accountID)
	if !strings.HasPrefix(arn, prefix) {
		return ""
	}
	return strings.TrimPrefix(arn, prefix)
}

func userXML(user User) string {
	return fmt.Sprintf("<Arn>%s</Arn><UserId>%s</UserId><UserName>%s</UserName>", user.Arn, user.UserId, user.UserName)
}

func roleXML(role Role) string {
	return fmt.Sprintf("<Arn>%s</Arn><AssumeRolePolicyDocument>%s</AssumeRolePolicyDocument><RoleId>%s</RoleId><RoleName>%s</RoleName>", role.Arn, role.AssumeRolePolicyDocument, role.RoleId, role.RoleName)
}

func policyXML(policy Policy) string {
	return fmt.Sprintf("<Arn>%s</Arn><PolicyName>%s</PolicyName><PolicyDocument>%s</PolicyDocument>", policy.PolicyArn, policy.PolicyName, policy.Document)
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", xmlContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<?xml version=\"1.0\"?>" + body))
}

func writeError(w http.ResponseWriter, code, message string) {
	writeXML(w, fmt.Sprintf("<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-error</RequestId></ErrorResponse>", code, message))
}
