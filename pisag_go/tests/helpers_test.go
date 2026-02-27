package tests

import (
	"net"
	"strconv"
)

func splitHostPort(hostPort string) (string, int, error) {
	h, p, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", 0, err
	}
	pi, _ := strconv.Atoi(p)
	return h, pi, nil
}
