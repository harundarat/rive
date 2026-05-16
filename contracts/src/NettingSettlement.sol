// SPDX-License-Identifier: MIT
pragma solidity 0.8.33;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title Rive Netting Settlement
/// @notice Pulls net debits from debtor agents and pays net credits to creditor agents.
contract NettingSettlement is ReentrancyGuard {
    using SafeERC20 for IERC20;

    IERC20 public immutable token;
    address public immutable settler;

    mapping(bytes32 batchHash => bool settled) public settledBatches;

    event BatchSettled(
        bytes32 indexed batchHash,
        uint256 debtorCount,
        uint256 creditorCount,
        uint256 totalAmount
    );

    error ZeroAddress();
    error NotSettler();
    error InvalidBatchHash();
    error InvalidArrayLength();
    error InvalidAmount();
    error TotalsMismatch();
    error BatchAlreadySettled();

    constructor(IERC20 _token, address _settler) {
        if (address(_token) == address(0) || _settler == address(0)) revert ZeroAddress();

        token = _token;
        settler = _settler;
    }

    function settleBatch(
        bytes32 batchHash,
        address[] calldata debtors,
        uint256[] calldata debitAmounts,
        address[] calldata creditors,
        uint256[] calldata creditAmounts
    ) external nonReentrant {
        if (msg.sender != settler) revert NotSettler();
        if (batchHash == bytes32(0)) revert InvalidBatchHash();
        if (settledBatches[batchHash]) revert BatchAlreadySettled();
        if (
            debtors.length == 0 || creditors.length == 0 || debtors.length != debitAmounts.length
                || creditors.length != creditAmounts.length
        ) {
            revert InvalidArrayLength();
        }

        uint256 debitTotal;
        for (uint256 i = 0; i < debtors.length; i++) {
            address debtor = debtors[i];
            uint256 amount = debitAmounts[i];
            if (debtor == address(0)) revert ZeroAddress();
            if (amount == 0) revert InvalidAmount();

            debitTotal += amount;
        }

        uint256 creditTotal;
        for (uint256 i = 0; i < creditors.length; i++) {
            address creditor = creditors[i];
            uint256 amount = creditAmounts[i];
            if (creditor == address(0)) revert ZeroAddress();
            if (amount == 0) revert InvalidAmount();

            creditTotal += amount;
        }

        if (debitTotal != creditTotal) revert TotalsMismatch();

        settledBatches[batchHash] = true;

        for (uint256 i = 0; i < debtors.length; i++) {
            token.safeTransferFrom(debtors[i], address(this), debitAmounts[i]);
        }

        for (uint256 i = 0; i < creditors.length; i++) {
            token.safeTransfer(creditors[i], creditAmounts[i]);
        }

        emit BatchSettled(batchHash, debtors.length, creditors.length, debitTotal);
    }
}
