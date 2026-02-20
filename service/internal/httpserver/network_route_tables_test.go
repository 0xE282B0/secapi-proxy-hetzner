package httpserver

import (
	"reflect"
	"testing"
)

func TestInternetGatewayNamesFromRoutes(t *testing.T) {
	t.Parallel()

	routes := []routeTableRouteSpec{
		{TargetRef: refObject{Resource: "internet-gateways/igw-b"}},
		{TargetRef: refObject{Resource: "instances/srv-a"}},
		{TargetRef: refObject{Resource: "internet-gateways/igw-a"}},
		{TargetRef: refObject{Resource: "internet-gateways/igw-a"}},
	}

	got := internetGatewayNamesFromRoutes(routes)
	want := []string{"igw-a", "igw-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected names: got=%v want=%v", got, want)
	}
}

func TestAffectedInternetGatewayNamesUnionCurrentAndPrevious(t *testing.T) {
	t.Parallel()

	current := []routeTableRouteSpec{
		{TargetRef: refObject{Resource: "internet-gateways/igw-new"}},
	}
	previous := []routeTableRouteSpec{
		{TargetRef: refObject{Resource: "internet-gateways/igw-old"}},
	}

	got := affectedInternetGatewayNames(current, previous)
	want := []string{"igw-new", "igw-old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected names: got=%v want=%v", got, want)
	}
}
