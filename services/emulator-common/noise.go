package emulatorcommon

import (
	"math/rand"
	"time"
)

type NoiseConfig struct {
	MissingPct     float64
	DuplicatePct   float64
	WrongAmountPct float64
	WrongFeePct    float64
	DelayMinMs     int
	DelayMaxMs     int
}

func ApplyNoise(event *PaymentTransactionEvent, cfg *NoiseConfig) (shouldSend bool, shouldDuplicate bool) {
	shouldSend = true
	shouldDuplicate = false


	if rand.Float64() < cfg.MissingPct {
		return false, false
	}


	if rand.Float64() < cfg.WrongAmountPct {
		event.AmountMinor = mutateAmount(event.AmountMinor)
	}


	if rand.Float64() < cfg.WrongFeePct {
		event.FeeMinor = mutateFee(event.FeeMinor)
	}


	if rand.Float64() < cfg.DuplicatePct {
		shouldDuplicate = true
	}


	if cfg.DelayMaxMs > 0 {
		delayMs := cfg.DelayMinMs
		if cfg.DelayMaxMs > cfg.DelayMinMs {
			delayMs += rand.Intn(cfg.DelayMaxMs - cfg.DelayMinMs)
		}
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	return shouldSend, shouldDuplicate
}

func mutateAmount(amount int64) int64 {

	factor := 0.9 + rand.Float64()*0.2
	return int64(float64(amount) * factor)
}

func mutateFee(fee int64) int64 {

	factor := 0.5 + rand.Float64()
	return int64(float64(fee) * factor)
}
