// Copyright Tharsis Labs Ltd.(Evmos)
// SPDX-License-Identifier:ENCL-1.0(https://github.com/evmos/evmos/blob/main/LICENSE)
package factory

import (
	"encoding/json"
	"errors"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/evmos/v16/server/config"
	evmtypes "github.com/evmos/evmos/v16/x/evm/types"
)

func (tf *IntegrationTxFactory) GenerateDefaultTxTypeArgs(sender common.Address, txType int) (evmtypes.EvmTxArgs, error) {
	defaultArgs := evmtypes.EvmTxArgs{}
	switch txType {
	case gethtypes.DynamicFeeTxType:
		return tf.populateEvmTxArgs(sender, defaultArgs)
	case gethtypes.AccessListTxType:
		defaultArgs.Accesses = &gethtypes.AccessList{{
			Address:     sender,
			StorageKeys: []common.Hash{{0}},
		}}
		defaultArgs.GasPrice = big.NewInt(1e9)
		return tf.populateEvmTxArgs(sender, defaultArgs)
	case gethtypes.LegacyTxType:
		defaultArgs.GasPrice = big.NewInt(1e9)
		return tf.populateEvmTxArgs(sender, defaultArgs)
	default:
		return evmtypes.EvmTxArgs{}, errors.New("tx type not supported")
	}
}

// EstimateGasLimit estimates the gas limit for a tx with the provided address and txArgs
func (tf *IntegrationTxFactory) EstimateGasLimit(from *common.Address, txArgs *evmtypes.EvmTxArgs) (uint64, error) {
	args, err := json.Marshal(evmtypes.TransactionArgs{
		Data:       (*hexutil.Bytes)(&txArgs.Input),
		From:       from,
		To:         txArgs.To,
		AccessList: txArgs.Accesses,
	})
	if err != nil {
		return 0, errorsmod.Wrap(err, "failed to marshal tx args")
	}

	res, err := tf.grpcHandler.EstimateGas(args, config.DefaultGasCap)
	if err != nil {
		return 0, errorsmod.Wrap(err, "failed to estimate gas")
	}
	gas := res.Gas
	return gas, nil
}

// GenerateSignedEthTx generates an Ethereum tx with the provided private key and txArgs but does not broadcast it.
func (tf *IntegrationTxFactory) GenerateSignedEthTx(privKey cryptotypes.PrivKey, txArgs evmtypes.EvmTxArgs) (signing.Tx, error) {
	signedMsg, err := tf.GenerateSignedMsgEthereumTx(privKey, txArgs)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to generate signed MsgEthereumTx")
	}

	// Validate the transaction to avoid unrealistic behavior
	if err = signedMsg.ValidateBasic(); err != nil {
		return nil, errorsmod.Wrap(err, "failed to validate transaction")
	}

	return tf.buildSignedTx(signedMsg)
}

// GenerateSignedMsgEthereumTx generates an MsgEthereumTx signed with the provided private key and txArgs.
func (tf *IntegrationTxFactory) GenerateSignedMsgEthereumTx(privKey cryptotypes.PrivKey, txArgs evmtypes.EvmTxArgs) (evmtypes.MsgEthereumTx, error) {
	msgEthereumTx, err := tf.GenerateMsgEthereumTx(privKey, txArgs)
	if err != nil {
		return evmtypes.MsgEthereumTx{}, errorsmod.Wrap(err, "failed to create ethereum tx")
	}

	return tf.SignMsgEthereumTx(privKey, msgEthereumTx)
}

// GenerateMsgEthereumTx creates a new MsgEthereumTx with the provided arguments.
// If any of the arguments are not provided, they will be populated with default values.
func (tf *IntegrationTxFactory) GenerateMsgEthereumTx(
	privKey cryptotypes.PrivKey,
	txArgs evmtypes.EvmTxArgs,
) (evmtypes.MsgEthereumTx, error) {
	fromAddr := common.BytesToAddress(privKey.PubKey().Address().Bytes())
	// Fill TxArgs with default values
	txArgs, err := tf.populateEvmTxArgs(fromAddr, txArgs)
	if err != nil {
		return evmtypes.MsgEthereumTx{}, errorsmod.Wrap(err, "failed to populate tx args")
	}
	msg := buildMsgEthereumTx(txArgs, fromAddr)

	return msg, nil
}

// GenerateGethCoreMsg creates a new GethCoreMsg with the provided arguments.
func (tf *IntegrationTxFactory) GenerateGethCoreMsg(
	privKey cryptotypes.PrivKey,
	txArgs evmtypes.EvmTxArgs,
) (core.Message, error) {
	msg, err := tf.GenerateMsgEthereumTx(privKey, txArgs)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to generate ethereum tx")
	}

	signedMsg, err := tf.SignMsgEthereumTx(privKey, msg)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to sign ethereum tx")
	}

	baseFeeResp, err := tf.grpcHandler.GetBaseFee()
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to get base fee")
	}
	signer := gethtypes.LatestSignerForChainID(
		tf.network.GetEIP155ChainID(),
	)
	return signedMsg.AsMessage(signer, baseFeeResp.BaseFee.BigInt())
}
