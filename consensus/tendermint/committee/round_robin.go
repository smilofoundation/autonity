package committee

import (
	"github.com/clearmatics/autonity/common"
	"github.com/clearmatics/autonity/core/types"
	"math/big"
)

func roundRobinProposer(valSet Set, proposer common.Address, round int64, height *big.Int) types.CommitteeMember {
	size := valSet.Size()
	seed := int(round)
	if proposer != (common.Address{}) {
		seed = calcSeed(valSet, proposer, round) + 1
	}
	pick := seed % size
	selectedProposer, _ := valSet.GetByIndex(pick)
	return selectedProposer
}
