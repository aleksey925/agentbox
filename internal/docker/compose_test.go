package docker

import (
	"testing"
)

func TestParseContainersOutput(t *testing.T) {
	// arrange
	output := "abc123def456\tmy-project-agentbox-1\t2 hours ago\n" +
		"789xyz000111\tother-agentbox-1\t5 minutes ago"

	// act
	containers := parseContainersOutput(output)

	// assert
	expected := []Container{
		{ID: "abc123def456", Name: "my-project-agentbox-1", Started: "2 hours ago"},
		{ID: "789xyz000111", Name: "other-agentbox-1", Started: "5 minutes ago"},
	}

	if len(containers) != len(expected) {
		t.Fatalf("len(containers) = %d, want %d", len(containers), len(expected))
	}

	for i, c := range containers {
		if c != expected[i] {
			t.Errorf("containers[%d] = %+v, want %+v", i, c, expected[i])
		}
	}
}

func TestParseContainersOutput__empty(t *testing.T) {
	// act
	containers := parseContainersOutput("")

	// assert
	if len(containers) != 0 {
		t.Errorf("len(containers) = %d, want 0", len(containers))
	}
}

func TestParseContainersOutput__incomplete_line(t *testing.T) {
	// arrange
	output := "abc123\tmy-project-agentbox-1"

	// act
	containers := parseContainersOutput(output)

	// assert
	if len(containers) != 0 {
		t.Errorf("incomplete lines should be skipped, got %d containers", len(containers))
	}
}

func TestParseContainersOutput__mixed_valid_invalid(t *testing.T) {
	// arrange
	output := "abc123def456\tmy-project-agentbox-1\t2 hours ago\n" +
		"incomplete\tline\n" +
		"789xyz000111\tother-agentbox-1\t5 minutes ago"

	// act
	containers := parseContainersOutput(output)

	// assert
	expected := []Container{
		{ID: "abc123def456", Name: "my-project-agentbox-1", Started: "2 hours ago"},
		{ID: "789xyz000111", Name: "other-agentbox-1", Started: "5 minutes ago"},
	}

	if len(containers) != len(expected) {
		t.Fatalf("len(containers) = %d, want %d", len(containers), len(expected))
	}

	for i, c := range containers {
		if c != expected[i] {
			t.Errorf("containers[%d] = %+v, want %+v", i, c, expected[i])
		}
	}
}

func TestParseContainersOutput__single_container(t *testing.T) {
	// arrange
	output := "abc123def456\tmy-project-agentbox-1\t2 hours ago"

	// act
	containers := parseContainersOutput(output)

	// assert
	if len(containers) != 1 {
		t.Fatalf("len(containers) = %d, want 1", len(containers))
	}

	expected := Container{ID: "abc123def456", Name: "my-project-agentbox-1", Started: "2 hours ago"}
	if containers[0] != expected {
		t.Errorf("containers[0] = %+v, want %+v", containers[0], expected)
	}
}
