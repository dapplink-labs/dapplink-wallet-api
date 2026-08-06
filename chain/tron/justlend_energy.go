package tron

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	justLendStrxAPIURL           = "https://openapi.just.network/lend/strx"
	minJustLendDelegatedSun      = int64(1_000_000)  // 1 TRX
	minJustLendLiquidationFeeSun = int64(20_000_000) // 20 TRX
	defaultStakeTrxPer10KEnergy  = 1045.0            // mainnet typical stake price when strx API is unavailable
)

type strxRentInfoResponse struct {
	Data struct {
		RentInfo struct {
			PriceFor10KEnergByRent  json.Number `json:"priceFor10KEnergByRent"`
			PriceFor10KEnergByStake json.Number `json:"priceFor10KEnergByStake"`
		} `json:"rentInfo"`
	} `json:"data"`
}

func fetchStrxRentInfo() (rentPrice, stakePrice float64, err error) {
	client := resty.New().
		SetTimeout(10 * time.Second).
		SetRetryCount(2)

	var payload strxRentInfoResponse
	resp, err := client.R().
		SetResult(&payload).
		Get(justLendStrxAPIURL)
	if err != nil {
		return 0, 0, err
	}
	if resp.IsError() {
		return 0, 0, fmt.Errorf("strx api status %d", resp.StatusCode())
	}

	rentPrice, err = payload.Data.RentInfo.PriceFor10KEnergByRent.Float64()
	if err != nil || rentPrice <= 0 {
		return 0, 0, fmt.Errorf("invalid priceFor10KEnergByRent")
	}
	stakePrice, err = payload.Data.RentInfo.PriceFor10KEnergByStake.Float64()
	if err != nil || stakePrice <= 0 {
		return 0, 0, fmt.Errorf("invalid priceFor10KEnergByStake")
	}
	return rentPrice, stakePrice, nil
}

func fetchStakeTrxPer10KEnergy() (float64, error) {
	_, stakePrice, err := fetchStrxRentInfo()
	return stakePrice, err
}

func EnergyToDelegatedSun(targetEnergy int64) (int64, error) {
	if targetEnergy <= 0 {
		return 0, fmt.Errorf("invalid target energy: %d", targetEnergy)
	}

	trxPer10K, err := fetchStakeTrxPer10KEnergy()
	if err != nil || trxPer10K <= 0 {
		trxPer10K = defaultStakeTrxPer10KEnergy
	}
	return energyToDelegatedSunWithStakePrice(targetEnergy, trxPer10K)
}

func energyToDelegatedSunWithStakePrice(targetEnergy int64, trxPer10KEnergy float64) (int64, error) {
	if targetEnergy <= 0 {
		return 0, fmt.Errorf("invalid target energy: %d", targetEnergy)
	}
	if trxPer10KEnergy <= 0 {
		trxPer10KEnergy = defaultStakeTrxPer10KEnergy
	}
	trxNeeded := float64(targetEnergy) / 10_000 * trxPer10KEnergy
	sun := int64(math.Ceil(trxNeeded * 1_000_000))
	if sun < minJustLendDelegatedSun {
		sun = minJustLendDelegatedSun
	}
	return sun, nil
}
