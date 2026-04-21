// SPDX-License-Identifier: MIT
pragma solidity 0.8.33;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title Rive Escrow — Trustless escrow for Work Orders between agents
/// @notice V1: manual release by payer + permissionless refund after timeout.
///         Verification layer (TEE/LLM-as-a-judge) deferred to V2.
contract Escrow is ReentrancyGuard {
    using SafeERC20 for IERC20;

    /*//////////////////////////////////////////////////////////////
                               CONSTANTS
    //////////////////////////////////////////////////////////////*/

    uint256 public constant REFUND_TIMEOUT = 7 days;

    /*//////////////////////////////////////////////////////////////
                                STORAGE
    //////////////////////////////////////////////////////////////*/

    IERC20 public immutable token;

    enum OrderState {
        None,
        Created,
        Funded,
        Released,
        Refunded
    }

    /// @dev Layout packed: slot 0 = payer (20) + fundedAt (8) + state (1),
    ///      slot 1 = payee, slot 2 = amount, slot 3 = specHash.
    struct Order {
        address payer;
        uint64 fundedAt;
        OrderState state;
        address payee;
        uint256 amount;
        bytes32 specHash; // 0G Storage merkle root for the Work Order spec
    }

    uint256 public nextOrderID;
    mapping(uint256 orderID => Order) public orders;

    /*//////////////////////////////////////////////////////////////
                                 EVENTS
    //////////////////////////////////////////////////////////////*/

    event OrderCreated(
        uint256 indexed orderID,
        address indexed payer,
        address indexed payee,
        uint256 amount,
        bytes32 specHash
    );
    event OrderFunded(uint256 indexed orderID, address indexed payer, uint256 amount);
    event OrderReleased(uint256 indexed orderID, address indexed payee, uint256 amount);
    event OrderRefunded(uint256 indexed orderID, address indexed payer, uint256 amount);

    /*//////////////////////////////////////////////////////////////
                                 ERRORS
    //////////////////////////////////////////////////////////////*/

    error ZeroAddress();
    error InvalidAmount();
    error InvalidState(OrderState current);
    error NotPayer();
    error TimeoutNotReached();

    /*//////////////////////////////////////////////////////////////
                              CONSTRUCTOR
    //////////////////////////////////////////////////////////////*/

    constructor(IERC20 _token) {
        if (address(_token) == address(0)) revert ZeroAddress();
        token = _token;
    }

    /*//////////////////////////////////////////////////////////////
                              CORE LOGIC
    //////////////////////////////////////////////////////////////*/

    /// @notice Create a Work Order. No funds are transferred yet — payer must
    ///         call `fundOrder` afterwards (with a prior `approve` allowance).
    function createOrder(address payee, uint256 amount, bytes32 specHash)
        external
        returns (uint256 orderID)
    {
        if (payee == address(0)) revert ZeroAddress();
        if (amount == 0) revert InvalidAmount();

        orderID = nextOrderID++;
        orders[orderID] = Order({
            payer: msg.sender,
            fundedAt: 0,
            state: OrderState.Created,
            payee: payee,
            amount: amount,
            specHash: specHash
        });

        emit OrderCreated(orderID, msg.sender, payee, amount, specHash);
    }

    /// @notice Lock funds from payer into escrow. Payer must `approve` this
    ///         contract for `amount` tokens beforehand.
    function fundOrder(uint256 orderID) external nonReentrant {
        Order storage order = orders[orderID];
        if (order.state != OrderState.Created) revert InvalidState(order.state);
        if (msg.sender != order.payer) revert NotPayer();

        order.state = OrderState.Funded;
        order.fundedAt = uint64(block.timestamp);

        uint256 amount = order.amount;
        token.safeTransferFrom(msg.sender, address(this), amount);

        emit OrderFunded(orderID, msg.sender, amount);
    }

    /// @notice Payer confirms delivery → funds are released to payee.
    function releaseOrder(uint256 orderID) external nonReentrant {
        Order storage order = orders[orderID];
        if (order.state != OrderState.Funded) revert InvalidState(order.state);
        if (msg.sender != order.payer) revert NotPayer();

        order.state = OrderState.Released;

        address payee = order.payee;
        uint256 amount = order.amount;
        token.safeTransfer(payee, amount);

        emit OrderReleased(orderID, payee, amount);
    }

    /// @notice After timeout, refund to payer. Permissionless: anyone can
    ///         trigger it (funds only return to `order.payer`), so that
    ///         keepers/bots can auto-refund if the payer is offline.
    function refundOrder(uint256 orderID) external nonReentrant {
        Order storage order = orders[orderID];
        if (order.state != OrderState.Funded) revert InvalidState(order.state);
        if (block.timestamp < uint256(order.fundedAt) + REFUND_TIMEOUT) {
            revert TimeoutNotReached();
        }

        order.state = OrderState.Refunded;

        address payer = order.payer;
        uint256 amount = order.amount;
        token.safeTransfer(payer, amount);

        emit OrderRefunded(orderID, payer, amount);
    }
}