// SPDX-License-Identifier: MIT
pragma solidity 0.8.33;

import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";

contract RiveUSD is ERC20 {
    constructor() ERC20("Rive USD", "rUSD") {}

    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }
}
