package main

import "testing"

func TestParseDockerInspectPublishedContainer(t *testing.T) {
	data := []byte(`[
        {
            "Id": "container-id",
            "Name": "/portly-postgres-1",
            "Config": {"Labels": {
                "com.docker.compose.project": "portly",
                "com.docker.compose.service": "postgres"
            }},
            "NetworkSettings": {"Ports": {
                "5432/tcp": [{"HostPort": "15432"}]
            }}
        }
    ]`)

	container := parseDockerInspect(data, 15432)
	if container == nil {
		t.Fatal("expected the published Docker container to be detected")
	}
	if container.ID != "container-id" || container.displayName() != "portly / postgres" {
		t.Fatalf("container=%+v", container)
	}
}

func TestParseDockerInspectIgnoresOtherPublishedPorts(t *testing.T) {
	data := []byte(`[
        {
            "Id": "container-id",
            "Name": "/other",
            "NetworkSettings": {"Ports": {
                "5432/tcp": [{"HostPort": "5433"}]
            }}
        }
    ]`)

	if container := parseDockerInspect(data, 15432); container != nil {
		t.Fatalf("unexpected container=%+v", container)
	}
}
