package mcp

import (
	"context"
	"errors"
	"testing"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestCallOfficialSDKDiscoveryPageV1CallsExactlyOnePage(t *testing.T) {
	session := completeFakeOfficialSDKSessionV1()
	observation, err := callOfficialSDKDiscoveryPageV1(context.Background(), session, toolcontract.MCPDiscoveryPageCommandV1{Namespace: runtimeports.MCPDiscoveryPageToolsNamespaceV1})
	if err != nil {
		t.Fatal(err)
	}
	if session.toolCalls != 1 || session.resourceCalls != 0 || session.promptCalls != 0 || len(observation.Tools) != 1 || observation.Tools[0].Name != "zeta" || string(observation.NextCursor) != "tools-2" || observation.Digest == "" {
		t.Fatalf("single Tool page call drifted: calls=%d/%d/%d observation=%#v", session.toolCalls, session.resourceCalls, session.promptCalls, observation)
	}
	if _, err = callOfficialSDKDiscoveryPageV1(context.Background(), session, toolcontract.MCPDiscoveryPageCommandV1{Namespace: runtimeports.MCPDiscoveryPageResourcesNamespaceV1}); err != nil || session.resourceCalls != 1 {
		t.Fatalf("single Resource page call failed: calls=%d err=%v", session.resourceCalls, err)
	}
	if _, err = callOfficialSDKDiscoveryPageV1(context.Background(), session, toolcontract.MCPDiscoveryPageCommandV1{Namespace: runtimeports.MCPDiscoveryPagePromptsNamespaceV1}); err != nil || session.promptCalls != 1 {
		t.Fatalf("single Prompt page call failed: calls=%d err=%v", session.promptCalls, err)
	}
}

func TestCallOfficialSDKDiscoveryPageV1FailsClosed(t *testing.T) {
	t.Run("provider_error", func(t *testing.T) {
		session := completeFakeOfficialSDKSessionV1()
		session.err = errors.New("lost reply")
		if _, err := callOfficialSDKDiscoveryPageV1(context.Background(), session, toolcontract.MCPDiscoveryPageCommandV1{Namespace: runtimeports.MCPDiscoveryPageToolsNamespaceV1}); err == nil || session.toolCalls != 1 {
			t.Fatalf("provider error/call count=%v/%d", err, session.toolCalls)
		}
	})
	t.Run("nil_page", func(t *testing.T) {
		session := completeFakeOfficialSDKSessionV1()
		delete(session.tools, "")
		if _, err := callOfficialSDKDiscoveryPageV1(context.Background(), session, toolcontract.MCPDiscoveryPageCommandV1{Namespace: runtimeports.MCPDiscoveryPageToolsNamespaceV1}); err == nil {
			t.Fatal("nil page was accepted")
		}
	})
	t.Run("unknown_namespace", func(t *testing.T) {
		if _, err := callOfficialSDKDiscoveryPageV1(context.Background(), completeFakeOfficialSDKSessionV1(), toolcontract.MCPDiscoveryPageCommandV1{Namespace: "praxis.mcp/unknown"}); err == nil {
			t.Fatal("unknown namespace was accepted")
		}
	})
	t.Run("duplicate_tool_names", func(t *testing.T) {
		session := completeFakeOfficialSDKSessionV1()
		page := session.tools[""]
		page.Tools = append(page.Tools, page.Tools[0])
		if _, err := callOfficialSDKDiscoveryPageV1(context.Background(), session, toolcontract.MCPDiscoveryPageCommandV1{Namespace: runtimeports.MCPDiscoveryPageToolsNamespaceV1}); err == nil {
			t.Fatal("duplicate Tool names were accepted")
		}
	})
	t.Run("duplicate_resource_uris", func(t *testing.T) {
		session := completeFakeOfficialSDKSessionV1()
		page := session.resources[""]
		page.Resources = append(page.Resources, page.Resources[0])
		if _, err := callOfficialSDKDiscoveryPageV1(context.Background(), session, toolcontract.MCPDiscoveryPageCommandV1{Namespace: runtimeports.MCPDiscoveryPageResourcesNamespaceV1}); err == nil {
			t.Fatal("duplicate Resource URIs were accepted")
		}
	})
	t.Run("duplicate_prompt_names", func(t *testing.T) {
		session := completeFakeOfficialSDKSessionV1()
		page := session.prompts[""]
		page.Prompts = append(page.Prompts, page.Prompts[0])
		if _, err := callOfficialSDKDiscoveryPageV1(context.Background(), session, toolcontract.MCPDiscoveryPageCommandV1{Namespace: runtimeports.MCPDiscoveryPagePromptsNamespaceV1}); err == nil {
			t.Fatal("duplicate Prompt names were accepted")
		}
	})
}
