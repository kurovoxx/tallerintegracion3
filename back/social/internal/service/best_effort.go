package service

import (
	"context"
	"log"
	"time"
)

// bestEffortTimeout es el presupuesto máximo por integración externa
// (Calendar, Discord, Stream). Acota goroutines colgadas: pasado el plazo,
// el contexto se cancela y el runner termina aunque el cliente siga bloqueado.
// Variable (no const) para acortarla en tests.
var bestEffortTimeout = 30 * time.Second

// GoBestEffort ejecuta fn en background con tres garantías:
//  1. No bloqueante: corre en su propia goroutine, el llamador sigue de inmediato.
//  2. Anti-pánico: un panic en la integración se loguea, jamás tumba el servidor.
//  3. Acotado: pasado bestEffortTimeout el contexto se cancela y no hay leaks.
//
// name identifica la integración en los logs (ej. "meetings", "group-channel").
func GoBestEffort(name string, fn func(ctx context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("best-effort %s: panic recuperado: %v", name, r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), bestEffortTimeout)
		defer cancel()
		fn(ctx)
	}()
}

// RunBestEffortChild ejecuta una integración hija dentro de un fan-out ya
// en background: mismo contrato que GoBestEffort pero síncrono (el padre
// decide paralelismo y espera). Reporta true si terminó sin panic.
func RunBestEffortChild(name string, ctx context.Context, fn func(ctx context.Context)) (finished bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("best-effort %s: panic recuperado: %v", name, r)
			finished = false
		}
	}()
	childCtx, cancel := context.WithTimeout(ctx, bestEffortTimeout)
	defer cancel()
	fn(childCtx)
	return true
}
