// SPDX-License-Identifier: UNLICENSED
pragma solidity 0.8.33;

import {Test} from "forge-std/Test.sol";

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

import {Escrow} from "../src/Escrow.sol";
import {RiveUSD} from "../src/mocks/RiveUSD.sol";

contract EscrowTest is Test {
    RiveUSD internal token;
    Escrow internal escrow;

    address internal payer;
    address internal payee;
    address internal keeper;
    address internal attacker;

    uint256 internal constant DEFAULT_AMOUNT = 100e18;
    uint256 internal constant SECOND_AMOUNT = 55e18;
    uint256 internal constant INITIAL_BALANCE = 1_000_000e18;
    bytes32 internal constant DEFAULT_SPEC_HASH = keccak256("default-spec");

    event OrderCreated(
        uint256 indexed orderID,
        address indexed payer,
        address indexed payee,
        uint256 amount,
        bytes32 specHash
    );
    event OrderReleased(uint256 indexed orderID, address indexed payee, uint256 amount);
    event OrderRefunded(uint256 indexed orderID, address indexed payer, uint256 amount);

    function setUp() public {
        payer = makeAddr("payer");
        payee = makeAddr("payee");
        keeper = makeAddr("keeper");
        attacker = makeAddr("attacker");

        token = new RiveUSD();
        escrow = new Escrow(IERC20(address(token)));

        token.mint(payer, INITIAL_BALANCE);
        token.mint(attacker, INITIAL_BALANCE);
        token.mint(keeper, 1e18);

        vm.prank(payer);
        token.approve(address(escrow), type(uint256).max);

        vm.prank(attacker);
        token.approve(address(escrow), type(uint256).max);
    }

    function test_Constructor_SetsTokenAndStartsCounter() public view {
        assertEq(address(escrow.token()), address(token));
        assertEq(escrow.nextOrderID(), 0);
    }

    function test_RiveUSD_Metadata() public view {
        assertEq(token.name(), "Rive USD");
        assertEq(token.symbol(), "rUSD");
        assertEq(token.decimals(), 18);
    }

    function test_RiveUSD_PublicMint() public {
        address minter = makeAddr("minter");
        address recipient = makeAddr("recipient");
        uint256 amount = 123e18;

        vm.prank(minter);
        token.mint(recipient, amount);

        assertEq(token.balanceOf(recipient), amount);
    }

    function test_Constructor_RevertsForZeroTokenAddress() public {
        vm.expectRevert(Escrow.ZeroAddress.selector);
        new Escrow(IERC20(address(0)));
    }

    function test_CreateOrder_TransfersFundsStoresFieldsAndEmits() public {
        uint256 payerBalanceBefore = token.balanceOf(payer);
        uint256 escrowBalanceBefore = token.balanceOf(address(escrow));

        vm.warp(1_700_000_000);

        vm.expectEmit(true, true, true, true, address(escrow));
        emit OrderCreated(0, payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);
        assertEq(orderID, 0);
        assertEq(escrow.nextOrderID(), 1);
        assertEq(token.balanceOf(payer), payerBalanceBefore - DEFAULT_AMOUNT);
        assertEq(token.balanceOf(address(escrow)), escrowBalanceBefore + DEFAULT_AMOUNT);

        (
            address orderPayer,
            uint64 createdAt,
            Escrow.OrderState state,
            address orderPayee,
            uint256 amount,
            bytes32 specHash
        ) = _getOrder(orderID);

        assertEq(orderPayer, payer);
        assertEq(createdAt, 1_700_000_000);
        assertEq(uint8(state), uint8(Escrow.OrderState.Created));
        assertEq(orderPayee, payee);
        assertEq(amount, DEFAULT_AMOUNT);
        assertEq(specHash, DEFAULT_SPEC_HASH);
    }

    function test_CreateOrder_IncrementsOrderID() public {
        uint256 first = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);
        uint256 second = _createOrder(attacker, keeper, SECOND_AMOUNT, bytes32(uint256(2)));

        assertEq(first, 0);
        assertEq(second, 1);
        assertEq(escrow.nextOrderID(), 2);
    }

    function test_CreateOrder_RevertsForZeroPayee() public {
        vm.prank(payer);
        vm.expectRevert(Escrow.ZeroAddress.selector);
        escrow.createOrder(address(0), DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);
    }

    function test_CreateOrder_RevertsForZeroAmount() public {
        vm.prank(payer);
        vm.expectRevert(Escrow.InvalidAmount.selector);
        escrow.createOrder(payee, 0, DEFAULT_SPEC_HASH);
    }

    function test_CreateOrder_RevertsWithoutAllowance() public {
        vm.prank(payer);
        token.approve(address(escrow), 0);

        vm.prank(payer);
        vm.expectRevert();
        escrow.createOrder(payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);
    }

    function test_CreateOrder_RevertsWithoutBalance() public {
        uint256 amount = INITIAL_BALANCE + 1;

        vm.prank(payer);
        vm.expectRevert();
        escrow.createOrder(payee, amount, DEFAULT_SPEC_HASH);
    }

    function testFuzz_CreateOrder_StoresArbitraryValidInput(
        address fuzzPayee,
        uint96 rawAmount,
        bytes32 specHash
    ) public {
        vm.assume(fuzzPayee != address(0));
        uint256 amount = bound(uint256(rawAmount), 1, INITIAL_BALANCE);

        uint256 payerBalanceBefore = token.balanceOf(payer);
        uint256 escrowBalanceBefore = token.balanceOf(address(escrow));

        uint256 orderID = _createOrder(payer, fuzzPayee, amount, specHash);
        (
            address orderPayer,
            uint64 createdAt,
            Escrow.OrderState state,
            address orderPayee,
            uint256 storedAmount,
            bytes32 storedSpecHash
        ) = _getOrder(orderID);

        assertEq(orderPayer, payer);
        assertEq(createdAt, uint64(block.timestamp));
        assertEq(uint8(state), uint8(Escrow.OrderState.Created));
        assertEq(orderPayee, fuzzPayee);
        assertEq(storedAmount, amount);
        assertEq(storedSpecHash, specHash);
        assertEq(token.balanceOf(payer), payerBalanceBefore - amount);
        assertEq(token.balanceOf(address(escrow)), escrowBalanceBefore + amount);
    }

    function test_ReleaseOrder_TransfersToPayeeSetsStateAndEmits() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        uint256 payeeBalanceBefore = token.balanceOf(payee);
        uint256 escrowBalanceBefore = token.balanceOf(address(escrow));

        vm.expectEmit(true, true, false, true, address(escrow));
        emit OrderReleased(orderID, payee, DEFAULT_AMOUNT);

        vm.prank(payer);
        escrow.releaseOrder(orderID);

        assertEq(token.balanceOf(payee), payeeBalanceBefore + DEFAULT_AMOUNT);
        assertEq(token.balanceOf(address(escrow)), escrowBalanceBefore - DEFAULT_AMOUNT);
        _assertState(orderID, Escrow.OrderState.Released);
    }

    function test_ReleaseOrder_RevertsForNonPayer() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        vm.prank(attacker);
        vm.expectRevert(Escrow.NotPayer.selector);
        escrow.releaseOrder(orderID);
    }

    function test_ReleaseOrder_RevertsForNonexistentOrder() public {
        vm.prank(payer);
        vm.expectRevert(abi.encodeWithSelector(Escrow.InvalidState.selector, Escrow.OrderState.None));
        escrow.releaseOrder(999);
    }

    function test_ReleaseOrder_RevertsWhenAlreadyReleased() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        vm.prank(payer);
        escrow.releaseOrder(orderID);

        vm.prank(payer);
        vm.expectRevert(abi.encodeWithSelector(Escrow.InvalidState.selector, Escrow.OrderState.Released));
        escrow.releaseOrder(orderID);
    }

    function test_RefundOrder_RevertsForNonexistentOrder() public {
        vm.prank(keeper);
        vm.expectRevert(abi.encodeWithSelector(Escrow.InvalidState.selector, Escrow.OrderState.None));
        escrow.refundOrder(999);
    }

    function test_RefundOrder_RevertsBeforeTimeout() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        (, uint64 createdAt, , , , ) = _getOrder(orderID);
        vm.warp(uint256(createdAt) + escrow.REFUND_TIMEOUT() - 1);

        vm.prank(keeper);
        vm.expectRevert(Escrow.TimeoutNotReached.selector);
        escrow.refundOrder(orderID);
    }

    function test_RefundOrder_AtExactTimeoutRefundsAndEmits() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        (, uint64 createdAt, , , , ) = _getOrder(orderID);
        vm.warp(uint256(createdAt) + escrow.REFUND_TIMEOUT());

        uint256 payerBalanceBefore = token.balanceOf(payer);
        uint256 escrowBalanceBefore = token.balanceOf(address(escrow));

        vm.expectEmit(true, true, false, true, address(escrow));
        emit OrderRefunded(orderID, payer, DEFAULT_AMOUNT);

        vm.prank(keeper);
        escrow.refundOrder(orderID);

        assertEq(token.balanceOf(payer), payerBalanceBefore + DEFAULT_AMOUNT);
        assertEq(token.balanceOf(address(escrow)), escrowBalanceBefore - DEFAULT_AMOUNT);
        _assertState(orderID, Escrow.OrderState.Refunded);
    }

    function test_RefundOrder_OneSecondBeforeTimeoutReverts() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        (, uint64 createdAt, , , , ) = _getOrder(orderID);
        vm.warp(uint256(createdAt) + escrow.REFUND_TIMEOUT() - 1);

        vm.prank(attacker);
        vm.expectRevert(Escrow.TimeoutNotReached.selector);
        escrow.refundOrder(orderID);
    }

    function test_RefundOrder_AnyCallerCanExecute() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        (, uint64 createdAt, , , , ) = _getOrder(orderID);
        vm.warp(uint256(createdAt) + escrow.REFUND_TIMEOUT());

        uint256 attackerBalanceBefore = token.balanceOf(attacker);
        uint256 payerBalanceBefore = token.balanceOf(payer);

        vm.prank(attacker);
        escrow.refundOrder(orderID);

        assertEq(token.balanceOf(attacker), attackerBalanceBefore);
        assertEq(token.balanceOf(payer), payerBalanceBefore + DEFAULT_AMOUNT);
        _assertState(orderID, Escrow.OrderState.Refunded);
    }

    function test_RefundOrder_RevertsWhenAlreadyRefunded() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        (, uint64 createdAt, , , , ) = _getOrder(orderID);
        vm.warp(uint256(createdAt) + escrow.REFUND_TIMEOUT());

        vm.prank(keeper);
        escrow.refundOrder(orderID);

        vm.prank(keeper);
        vm.expectRevert(abi.encodeWithSelector(Escrow.InvalidState.selector, Escrow.OrderState.Refunded));
        escrow.refundOrder(orderID);
    }

    function test_RefundOrder_WorksLongAfterTimeout() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        (, uint64 createdAt, , , , ) = _getOrder(orderID);
        vm.warp(uint256(createdAt) + escrow.REFUND_TIMEOUT() + 365 days);

        vm.prank(keeper);
        escrow.refundOrder(orderID);

        _assertState(orderID, Escrow.OrderState.Refunded);
    }

    function testFuzz_RefundOrder_AfterTimeoutAnyCallerNoReward(address caller, uint96 rawAmount) public {
        vm.assume(caller != address(0));
        vm.assume(caller != payer);
        vm.assume(caller != address(escrow));

        uint256 amount = bound(uint256(rawAmount), 1, INITIAL_BALANCE);
        uint256 orderID = _createOrder(payer, payee, amount, DEFAULT_SPEC_HASH);

        (, uint64 createdAt, , , , ) = _getOrder(orderID);
        vm.warp(uint256(createdAt) + escrow.REFUND_TIMEOUT());

        uint256 callerBalanceBefore = token.balanceOf(caller);
        uint256 payerBalanceBefore = token.balanceOf(payer);

        vm.prank(caller);
        escrow.refundOrder(orderID);

        assertEq(token.balanceOf(caller), callerBalanceBefore);
        assertEq(token.balanceOf(payer), payerBalanceBefore + amount);
        _assertState(orderID, Escrow.OrderState.Refunded);
    }

    function test_FullFlow_CreateRelease() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        vm.prank(payer);
        escrow.releaseOrder(orderID);

        assertEq(token.balanceOf(payee), DEFAULT_AMOUNT);
        assertEq(token.balanceOf(address(escrow)), 0);
        _assertState(orderID, Escrow.OrderState.Released);
    }

    function test_FullFlow_CreateRefund() public {
        uint256 orderID = _createOrder(payer, payee, DEFAULT_AMOUNT, DEFAULT_SPEC_HASH);

        (, uint64 createdAt, , , , ) = _getOrder(orderID);
        vm.warp(uint256(createdAt) + escrow.REFUND_TIMEOUT());

        vm.prank(keeper);
        escrow.refundOrder(orderID);

        assertEq(token.balanceOf(address(escrow)), 0);
        _assertState(orderID, Escrow.OrderState.Refunded);
    }

    function test_MultipleOrders_AreIsolatedAcrossTransitions() public {
        uint256 orderA = _createOrder(payer, payee, DEFAULT_AMOUNT, bytes32(uint256(1)));
        uint256 orderB = _createOrder(attacker, keeper, SECOND_AMOUNT, bytes32(uint256(2)));

        vm.prank(payer);
        escrow.releaseOrder(orderA);

        _assertState(orderA, Escrow.OrderState.Released);
        _assertState(orderB, Escrow.OrderState.Created);

        (, uint64 createdAtB, , , , ) = _getOrder(orderB);
        vm.warp(uint256(createdAtB) + escrow.REFUND_TIMEOUT());

        vm.prank(payer);
        escrow.refundOrder(orderB);

        _assertState(orderA, Escrow.OrderState.Released);
        _assertState(orderB, Escrow.OrderState.Refunded);
    }

    function test_Orders_NonexistentOrderReturnsDefaults() public view {
        (
            address orderPayer,
            uint64 createdAt,
            Escrow.OrderState state,
            address orderPayee,
            uint256 amount,
            bytes32 specHash
        ) = _getOrder(999_999);

        assertEq(orderPayer, address(0));
        assertEq(createdAt, 0);
        assertEq(uint8(state), uint8(Escrow.OrderState.None));
        assertEq(orderPayee, address(0));
        assertEq(amount, 0);
        assertEq(specHash, bytes32(0));
    }

    function _createOrder(address orderPayer, address orderPayee, uint256 amount, bytes32 specHash)
        internal
        returns (uint256 orderID)
    {
        vm.prank(orderPayer);
        orderID = escrow.createOrder(orderPayee, amount, specHash);
    }

    function _assertState(uint256 orderID, Escrow.OrderState expectedState) internal view {
        (, , Escrow.OrderState currentState, , , ) = _getOrder(orderID);
        assertEq(uint8(currentState), uint8(expectedState));
    }

    function _getOrder(uint256 orderID)
        internal
        view
        returns (
            address orderPayer,
            uint64 createdAt,
            Escrow.OrderState state,
            address orderPayee,
            uint256 amount,
            bytes32 specHash
        )
    {
        return escrow.orders(orderID);
    }
}
