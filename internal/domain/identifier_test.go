package domain

import (
	"errors"
	"testing"
)

func TestNormalizeResourceID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "normalizes case and spaces", input: "  Voyage-Alpha-1 ", want: "voyage-alpha-1"},
		{name: "accepts compact id", input: "abc", want: "abc"},
		{name: "rejects leading number", input: "1-voyage", wantErr: true},
		{name: "rejects underscore", input: "voyage_one", wantErr: true},
		{name: "rejects repeated separator", input: "voyage--one", wantErr: true},
		{name: "rejects too short", input: "a", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeResourceID(test.input)
			if test.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("error=%v", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}

func TestNormalizeTransportCodes(t *testing.T) {
	port, err := NormalizePortCode(" cnqzh ")
	if err != nil || port != "CNQZH" {
		t.Fatalf("port=%q err=%v", port, err)
	}
	imo, err := NormalizeIMO(" imo7654321 ")
	if err != nil || imo != "IMO7654321" {
		t.Fatalf("imo=%q err=%v", imo, err)
	}
	for _, invalid := range []string{"CN-QZH", "QZH", "CNQZH1"} {
		if _, err := NormalizePortCode(invalid); !errors.Is(err, ErrInvalid) {
			t.Errorf("port %q error=%v", invalid, err)
		}
	}
	for _, invalid := range []string{"IMO123", "1234567", "IMO12345678"} {
		if _, err := NormalizeIMO(invalid); !errors.Is(err, ErrInvalid) {
			t.Errorf("IMO %q error=%v", invalid, err)
		}
	}
}

func TestNormalizeContainerNumberUsesISOCheckDigit(t *testing.T) {
	valid, err := NormalizeContainerNumber("csqu 3054383")
	if err != nil || valid != "CSQU3054383" {
		t.Fatalf("container=%q err=%v", valid, err)
	}
	invalid := []string{"CSQU3054384", "CSQ3054383", "csqu-3054383", ""}
	for _, value := range invalid {
		if _, err := NormalizeContainerNumber(value); !errors.Is(err, ErrInvalid) {
			t.Errorf("container %q error=%v", value, err)
		}
	}
}
