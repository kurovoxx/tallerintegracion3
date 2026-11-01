// Test del ciclo de vida del gRPC en main (cierre 3_4_2): serveGRPC marca
// sirviendo al arrancar y registra el error al detenerse, que es lo que lee
// /health/db. Sin PostgreSQL ni red externa (127.0.0.1:0).
package main

import (
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestServeGRPCRegistraArranqueYDetencion(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	health := &grpcHealth{}
	done := make(chan struct{})
	go func() { serveGRPC(s, lis, "test", health); close(done) }()

	// Esperar a que esté sirviendo antes de detener (evita carrera).
	deadline := time.Now().Add(5 * time.Second)
	for {
		if serving, _ := health.snapshot(); serving {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("nunca marcó sirviendo")
		}
		time.Sleep(5 * time.Millisecond)
	}

	s.Stop()
	<-done
	if serving, _ := health.snapshot(); serving {
		t.Fatal("tras Stop debe marcar no-sirviendo")
	}

	fresh := &grpcHealth{}
	if serving, _ := fresh.snapshot(); serving {
		t.Fatal("antes de servir debe marcar no-sirviendo")
	}
	fresh.setServing()
	if serving, msg := fresh.snapshot(); !serving || msg != "" {
		t.Fatalf("al servir: serving=%v err=%q", serving, msg)
	}
	fresh.setFailed(errors.New("boom"))
	if serving, msg := fresh.snapshot(); serving || msg != "boom" {
		t.Fatalf("tras fallo: serving=%v err=%q", serving, msg)
	}
}
