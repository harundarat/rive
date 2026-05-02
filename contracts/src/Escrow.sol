// SPDX-License-Identifier: MIT
pragma solidity 0.8.33;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title  Rive Escrow - trustless escrow for Work Orders between AI agents
/// @author Rive V1
/// @notice Locks ERC20 funds for a Work Order until the payer releases or the timeout allows refund.
///
/// @dev    `Created` always means funded: order creation and token transfer are atomic.
///         The Work Order spec lives off-chain; this contract anchors only `specHash`.
contract Escrow is ReentrancyGuard {
    using SafeERC20 for IERC20;

    /*//////////////////////////////////////////////////////////////
                               CONSTANTS
    //////////////////////////////////////////////////////////////*/

    /// @notice Duration after which anyone may trigger a refund to the payer.
    uint256 public constant REFUND_TIMEOUT = 7 days;

    /*//////////////////////////////////////////////////////////////
                                STORAGE
    //////////////////////////////////////////////////////////////*/

    /// @notice ERC20 token used for all escrow payments in this deployment.
    IERC20 public immutable token;

    /// @notice Lifecycle states for a Work Order.
    enum OrderState {
        None,
        Created,
        Released,
        Refunded
    }

    /// @notice On-chain record of a Work Order.
    /// @param payer     Address that created and funded the order.
    /// @param createdAt Block timestamp recorded at creation for refund timeout checks.
    /// @param state     Current lifecycle state of the order.
    /// @param payee     Address that receives funds on release.
    /// @param amount    Token amount locked in escrow.
    /// @param specHash  Hash anchoring the off-chain Work Order spec.
    struct Order {
        address payer;
        uint64 createdAt;
        OrderState state;
        address payee;
        uint256 amount;
        bytes32 specHash;
    }

    /// @notice Monotonically increasing ID assigned to each new order.
    uint256 public nextOrderID;

    /// @notice Order records by ID.
    mapping(uint256 orderID => Order) public orders;

    /*//////////////////////////////////////////////////////////////
                                 EVENTS
    //////////////////////////////////////////////////////////////*/

    /// @notice Emitted when a Work Order is created and funded.
    /// @param orderID  Unique identifier for this order.
    /// @param payer    Address that locked the funds.
    /// @param payee    Address that will receive funds upon release.
    /// @param amount   Token amount locked (smallest unit).
    /// @param specHash 0G Storage Merkle root of the Work Order spec.
    event OrderCreated(
        uint256 indexed orderID,
        address indexed payer,
        address indexed payee,
        uint256 amount,
        bytes32 specHash
    );

    /// @notice Emitted when funds are released to the payee.
    /// @param orderID Unique identifier for this order.
    /// @param payee   Recipient of the released funds.
    /// @param amount  Token amount transferred.
    event OrderReleased(uint256 indexed orderID, address indexed payee, uint256 amount);

    /// @notice Emitted when funds are refunded to the payer.
    /// @param orderID Unique identifier for this order.
    /// @param payer   Recipient of the refunded funds.
    /// @param amount  Token amount returned.
    event OrderRefunded(uint256 indexed orderID, address indexed payer, uint256 amount);

    /*//////////////////////////////////////////////////////////////
                                 ERRORS
    //////////////////////////////////////////////////////////////*/

    /// @notice Thrown when a zero address is provided where one is not allowed.
    error ZeroAddress();

    /// @notice Thrown when `amount` is zero.
    error InvalidAmount();

    /// @notice Thrown when an order is not in the required state.
    /// @param current The actual current state of the order.
    error InvalidState(OrderState current);

    /// @notice Thrown when a caller is not the order's payer.
    error NotPayer();

    /// @notice Thrown when `refundOrder` is called before `REFUND_TIMEOUT`
    ///         has elapsed since the order was created.
    error TimeoutNotReached();

    /*//////////////////////////////////////////////////////////////
                              CONSTRUCTOR
    //////////////////////////////////////////////////////////////*/

    /// @param _token ERC20 token to use for all escrow payments.
    constructor(IERC20 _token) {
        if (address(_token) == address(0)) revert ZeroAddress();
        token = _token;
    }

    /*//////////////////////////////////////////////////////////////
                              CORE LOGIC
    //////////////////////////////////////////////////////////////*/

    /// @notice Create a Work Order and lock funds.
    /// @dev    Caller must have approved this contract for at least `amount`
    ///         tokens before calling. There is no intermediate unfunded state.
    ///
    /// @param payee    Address of the service provider (must not be zero).
    /// @param amount   Token amount to lock in escrow (must be > 0).
    /// @param specHash Merkle root anchoring the Work Order spec on 0G Storage.
    /// @return orderID The newly assigned order identifier.
    function createOrder(address payee, uint256 amount, bytes32 specHash)
        external
        nonReentrant
        returns (uint256 orderID)
    {
        if (payee == address(0)) revert ZeroAddress();
        if (amount == 0) revert InvalidAmount();

        orderID = nextOrderID++;
        orders[orderID] = Order({
            payer: msg.sender,
            createdAt: uint64(block.timestamp),
            state: OrderState.Created,
            payee: payee,
            amount: amount,
            specHash: specHash
        });

        token.safeTransferFrom(msg.sender, address(this), amount);

        emit OrderCreated(orderID, msg.sender, payee, amount, specHash);
    }

    /// @notice Confirm delivery and release escrowed funds to the payee.
    /// @dev    Only callable by the order's payer.
    /// @param orderID ID of the order to release.
    function releaseOrder(uint256 orderID) external nonReentrant {
        Order storage order = orders[orderID];
        if (order.state != OrderState.Created) revert InvalidState(order.state);
        if (msg.sender != order.payer) revert NotPayer();

        order.state = OrderState.Released;

        address payee = order.payee;
        uint256 amount = order.amount;
        token.safeTransfer(payee, amount);

        emit OrderReleased(orderID, payee, amount);
    }

    /// @notice Refund escrowed funds to the payer after `REFUND_TIMEOUT`.
    /// @dev    Intentionally permissionless: any address may call this function,
    ///         but funds always return to `order.payer`.
    /// @param orderID ID of the order to refund.
    function refundOrder(uint256 orderID) external nonReentrant {
        Order storage order = orders[orderID];
        if (order.state != OrderState.Created) revert InvalidState(order.state);
        if (block.timestamp < uint256(order.createdAt) + REFUND_TIMEOUT) {
            revert TimeoutNotReached();
        }

        order.state = OrderState.Refunded;

        address payer = order.payer;
        uint256 amount = order.amount;
        token.safeTransfer(payer, amount);

        emit OrderRefunded(orderID, payer, amount);
    }
}
