// SPDX-License-Identifier: UNLICENSED
pragma solidity 0.8.33;

import {Test} from "forge-std/Test.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

import {NettingSettlement} from "../src/NettingSettlement.sol";
import {RiveUSD} from "../src/mocks/RiveUSD.sol";

contract NettingSettlementTest is Test {
    RiveUSD internal token;
    NettingSettlement internal settlement;

    address internal settler;
    address internal nonSettler;
    address internal agentA;
    address internal agentB;
    address internal agentC;

    bytes32 internal constant DEFAULT_BATCH_HASH = keccak256("netting-batch-1");
    uint256 internal constant INITIAL_BALANCE = 1_000_000e18;

    event BatchSettled(
        bytes32 indexed batchHash,
        uint256 debtorCount,
        uint256 creditorCount,
        uint256 totalAmount
    );

    function setUp() public {
        settler = makeAddr("settler");
        nonSettler = makeAddr("nonSettler");
        agentA = makeAddr("agentA");
        agentB = makeAddr("agentB");
        agentC = makeAddr("agentC");

        token = new RiveUSD();
        settlement = new NettingSettlement(IERC20(address(token)), settler);

        token.mint(agentA, INITIAL_BALANCE);
        token.mint(agentB, INITIAL_BALANCE);
        token.mint(agentC, INITIAL_BALANCE);

        vm.prank(agentA);
        token.approve(address(settlement), type(uint256).max);

        vm.prank(agentB);
        token.approve(address(settlement), type(uint256).max);

        vm.prank(agentC);
        token.approve(address(settlement), type(uint256).max);
    }

    function test_Constructor_SetsTokenAndSettler() public view {
        assertEq(address(settlement.token()), address(token));
        assertEq(settlement.settler(), settler);
    }

    function test_Constructor_RevertsForZeroToken() public {
        vm.expectRevert(NettingSettlement.ZeroAddress.selector);
        new NettingSettlement(IERC20(address(0)), settler);
    }

    function test_Constructor_RevertsForZeroSettler() public {
        vm.expectRevert(NettingSettlement.ZeroAddress.selector);
        new NettingSettlement(IERC20(address(token)), address(0));
    }

    function test_SettleBatch_PullsFromDebtorsPaysCreditorsAndEmits() public {
        address[] memory debtors = new address[](1);
        debtors[0] = agentA;
        uint256[] memory debits = new uint256[](1);
        debits[0] = 90e18;

        address[] memory creditors = new address[](2);
        creditors[0] = agentB;
        creditors[1] = agentC;
        uint256[] memory credits = new uint256[](2);
        credits[0] = 80e18;
        credits[1] = 10e18;

        vm.expectEmit(true, false, false, true, address(settlement));
        emit BatchSettled(DEFAULT_BATCH_HASH, 1, 2, 90e18);

        vm.prank(settler);
        settlement.settleBatch(DEFAULT_BATCH_HASH, debtors, debits, creditors, credits);

        assertTrue(settlement.settledBatches(DEFAULT_BATCH_HASH));
        assertEq(token.balanceOf(agentA), INITIAL_BALANCE - 90e18);
        assertEq(token.balanceOf(agentB), INITIAL_BALANCE + 80e18);
        assertEq(token.balanceOf(agentC), INITIAL_BALANCE + 10e18);
        assertEq(token.balanceOf(address(settlement)), 0);
    }

    function test_SettleBatch_RevertsForNonSettler() public {
        (address[] memory debtors, uint256[] memory debits, address[] memory creditors, uint256[] memory credits) =
            _singleTransferBatch();

        vm.prank(nonSettler);
        vm.expectRevert(NettingSettlement.NotSettler.selector);
        settlement.settleBatch(DEFAULT_BATCH_HASH, debtors, debits, creditors, credits);
    }

    function test_SettleBatch_RevertsForTotalsMismatch() public {
        address[] memory debtors = new address[](1);
        debtors[0] = agentA;
        uint256[] memory debits = new uint256[](1);
        debits[0] = 90e18;

        address[] memory creditors = new address[](1);
        creditors[0] = agentB;
        uint256[] memory credits = new uint256[](1);
        credits[0] = 89e18;

        vm.prank(settler);
        vm.expectRevert(NettingSettlement.TotalsMismatch.selector);
        settlement.settleBatch(DEFAULT_BATCH_HASH, debtors, debits, creditors, credits);
    }

    function test_SettleBatch_RevertsForDuplicateBatchHash() public {
        (address[] memory debtors, uint256[] memory debits, address[] memory creditors, uint256[] memory credits) =
            _singleTransferBatch();

        vm.prank(settler);
        settlement.settleBatch(DEFAULT_BATCH_HASH, debtors, debits, creditors, credits);

        vm.prank(settler);
        vm.expectRevert(NettingSettlement.BatchAlreadySettled.selector);
        settlement.settleBatch(DEFAULT_BATCH_HASH, debtors, debits, creditors, credits);
    }

    function test_SettleBatch_RevertsForInvalidArrayLength() public {
        address[] memory debtors = new address[](1);
        debtors[0] = agentA;
        uint256[] memory debits = new uint256[](0);
        address[] memory creditors = new address[](1);
        creditors[0] = agentB;
        uint256[] memory credits = new uint256[](1);
        credits[0] = 10e18;

        vm.prank(settler);
        vm.expectRevert(NettingSettlement.InvalidArrayLength.selector);
        settlement.settleBatch(DEFAULT_BATCH_HASH, debtors, debits, creditors, credits);
    }

    function test_SettleBatch_RevertsForZeroAddress() public {
        (address[] memory debtors, uint256[] memory debits, address[] memory creditors, uint256[] memory credits) =
            _singleTransferBatch();
        creditors[0] = address(0);

        vm.prank(settler);
        vm.expectRevert(NettingSettlement.ZeroAddress.selector);
        settlement.settleBatch(DEFAULT_BATCH_HASH, debtors, debits, creditors, credits);
    }

    function test_SettleBatch_RevertsForZeroAmount() public {
        (address[] memory debtors, uint256[] memory debits, address[] memory creditors, uint256[] memory credits) =
            _singleTransferBatch();
        debits[0] = 0;
        credits[0] = 0;

        vm.prank(settler);
        vm.expectRevert(NettingSettlement.InvalidAmount.selector);
        settlement.settleBatch(DEFAULT_BATCH_HASH, debtors, debits, creditors, credits);
    }

    function _singleTransferBatch()
        internal
        view
        returns (
            address[] memory debtors,
            uint256[] memory debits,
            address[] memory creditors,
            uint256[] memory credits
        )
    {
        debtors = new address[](1);
        debtors[0] = agentA;
        debits = new uint256[](1);
        debits[0] = 10e18;
        creditors = new address[](1);
        creditors[0] = agentB;
        credits = new uint256[](1);
        credits[0] = 10e18;
    }
}
