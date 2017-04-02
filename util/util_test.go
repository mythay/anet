package util

import (
	"net"
	"reflect"
	"testing"
)

func Test_parseIPRange(t *testing.T) {
	type args struct {
		orig string
	}
	tests := []struct {
		name    string
		args    args
		want    []net.IP
		wantErr bool
	}{
		// TODO: Add test cases.
		{"basic ip", args{"192.168.1.1"}, []net.IP{net.ParseIP("192.168.1.1")}, false},
		{"basic ip", args{"192.168.1.255"}, []net.IP{net.ParseIP("192.168.1.255")}, false},
		{"basic ip", args{"192.168.1.2557"}, nil, true},
		{"basic ip", args{"192.168.1.1-2"}, []net.IP{net.ParseIP("192.168.1.1"), net.ParseIP("192.168.1.2")}, false},
		{"basic ip", args{"192.168.1.2-1"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIPRange(tt.args.orig)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIPRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseIPRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseMultipleIPRange(t *testing.T) {
	type args struct {
		ipstrs []string
	}
	tests := []struct {
		name    string
		args    args
		want    []net.IP
		wantErr bool
	}{
		{"basic ip", args{[]string{"192.168.1.1"}}, []net.IP{net.ParseIP("192.168.1.1")}, false},
		{"basic ip", args{[]string{"192.168.1.255"}}, []net.IP{net.ParseIP("192.168.1.255")}, false},
		{"basic ip", args{[]string{"192.168.1.2557"}}, nil, true},
		{"basic ip", args{[]string{"192.168.1.1-2"}}, []net.IP{net.ParseIP("192.168.1.1"), net.ParseIP("192.168.1.2")}, false},
		{"basic ip", args{[]string{"192.168.1.2-1"}}, nil, true},

		{"basic ip", args{[]string{"192.168.1.1", "192.168.1.1"}}, []net.IP{net.ParseIP("192.168.1.1")}, false},
		{"basic ip", args{[]string{"192.168.1.1", "1.1.1.1"}}, []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("192.168.1.1")}, false},
		{"basic ip", args{[]string{"192.168.1.2-3", "192.168.1.1-2"}}, []net.IP{net.ParseIP("192.168.1.1"), net.ParseIP("192.168.1.2"), net.ParseIP("192.168.1.3")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMultipleIPRange(tt.args.ipstrs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMultipleIPRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseMultipleIPRange() = %v, want %v", got, tt.want)
			}
		})
	}
}
