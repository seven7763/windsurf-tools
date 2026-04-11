package services

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func buildConnectEOSFrame(code, message string, gzipped bool) []byte {
	payload := ConnectEOSPayload{
		Error: &ConnectError{Code: code, Message: message},
	}
	jsonBytes, _ := json.Marshal(payload)

	var flag byte = 0x02 // EOS
	framePayload := jsonBytes
	if gzipped {
		flag |= 0x01
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		gw.Write(jsonBytes)
		gw.Close()
		framePayload = buf.Bytes()
	}

	frame := make([]byte, 5+len(framePayload))
	frame[0] = flag
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(framePayload)))
	copy(frame[5:], framePayload)
	return frame
}

func buildDataFrame(data []byte) []byte {
	frame := make([]byte, 5+len(data))
	frame[0] = 0x00 // data frame
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(data)))
	copy(frame[5:], data)
	return frame
}

func TestParseConnectEOS_BasicEOS(t *testing.T) {
	frame := buildConnectEOSFrame("resource_exhausted", "Your daily usage quota has been exhausted", false)
	result := ParseConnectEOS(frame)
	if !result.IsError {
		t.Fatal("expected error")
	}
	if !result.IsEOS {
		t.Fatal("expected EOS")
	}
	if result.Code != "resource_exhausted" {
		t.Fatalf("expected code resource_exhausted, got %s", result.Code)
	}
}

func TestParseConnectEOS_GzippedEOS(t *testing.T) {
	frame := buildConnectEOSFrame("resource_exhausted", "quota exhausted", true)
	result := ParseConnectEOS(frame)
	if !result.IsError {
		t.Fatal("expected error")
	}
	if result.Code != "resource_exhausted" {
		t.Fatalf("expected code resource_exhausted, got %s", result.Code)
	}
}

func TestParseConnectEOS_FailedPrecondition(t *testing.T) {
	frame := buildConnectEOSFrame("failed_precondition", "Invalid Cascade session, please try again", false)
	result := ParseConnectEOS(frame)
	if !result.IsError {
		t.Fatal("expected error")
	}
	if result.Code != "failed_precondition" {
		t.Fatalf("expected code failed_precondition, got %s", result.Code)
	}
	if !IsCascadeSessionError(result) {
		t.Fatal("expected cascade session error")
	}
}

func TestParseConnectEOS_DataThenEOS(t *testing.T) {
	dataFrame := buildDataFrame([]byte("some chat data"))
	eosFrame := buildConnectEOSFrame("resource_exhausted", "credits gone", false)
	body := append(dataFrame, eosFrame...)

	result := ParseConnectEOS(body)
	if !result.IsError {
		t.Fatal("expected error from EOS after data frame")
	}
	if result.Code != "resource_exhausted" {
		t.Fatalf("expected code resource_exhausted, got %s", result.Code)
	}
}

func TestParseConnectEOS_PlainJSON(t *testing.T) {
	body := []byte(`{"code":"permission_denied","message":"Permission denied"}`)
	result := ParseConnectEOS(body)
	if !result.IsError {
		t.Fatal("expected error")
	}
	if result.Code != "permission_denied" {
		t.Fatalf("expected code permission_denied, got %s", result.Code)
	}
}

func TestParseConnectEOS_WrappedJSON(t *testing.T) {
	body := []byte(`{"error":{"code":"unauthenticated","message":"token expired"}}`)
	result := ParseConnectEOS(body)
	if !result.IsError {
		t.Fatal("expected error")
	}
	if result.Code != "unauthenticated" {
		t.Fatalf("expected code unauthenticated, got %s", result.Code)
	}
}

func TestParseConnectEOS_NoError(t *testing.T) {
	body := []byte("just some random bytes")
	result := ParseConnectEOS(body)
	if result.IsError {
		t.Fatal("expected no error")
	}
}

func TestClassifyConnectError_Quota(t *testing.T) {
	ce := ConnectErrorResult{IsError: true, Code: "resource_exhausted", Message: "daily usage quota exhausted"}
	kind, _ := ClassifyConnectError(ce)
	if kind != upstreamFailureQuota {
		t.Fatalf("expected quota, got %s", kind)
	}
}

func TestClassifyConnectError_RateLimit(t *testing.T) {
	ce := ConnectErrorResult{IsError: true, Code: "resource_exhausted", Message: "rate limit exceeded"}
	kind, _ := ClassifyConnectError(ce)
	if kind != upstreamFailureRateLimit {
		t.Fatalf("expected rate_limit, got %s", kind)
	}
}

func TestClassifyConnectError_CascadeSession(t *testing.T) {
	ce := ConnectErrorResult{IsError: true, Code: "failed_precondition", Message: "Invalid Cascade session, please try again"}
	kind, _ := ClassifyConnectError(ce)
	if kind != upstreamFailureGRPC {
		t.Fatalf("expected grpc (cascade session), got %s", kind)
	}
	if !IsCascadeSessionError(ce) {
		t.Fatal("expected cascade session error")
	}
}

func TestClassifyConnectError_QuotaPrecondition(t *testing.T) {
	ce := ConnectErrorResult{IsError: true, Code: "failed_precondition", Message: "Your daily usage quota has been exhausted"}
	kind, _ := ClassifyConnectError(ce)
	if kind != upstreamFailureQuota {
		t.Fatalf("expected quota, got %s", kind)
	}
}

func TestClassifyConnectError_PermissionDenied(t *testing.T) {
	ce := ConnectErrorResult{IsError: true, Code: "permission_denied", Message: "Permission denied"}
	kind, _ := ClassifyConnectError(ce)
	if kind != upstreamFailurePermission {
		t.Fatalf("expected permission, got %s", kind)
	}
}
