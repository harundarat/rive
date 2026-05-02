// SPDX-License-Identifier: UNLICENSED
pragma solidity 0.8.33;

import {Script, console2} from "forge-std/Script.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

import {Escrow} from "../src/Escrow.sol";

contract DeployEscrowScript is Script {
    Escrow public escrow;

    function run() public {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address token = vm.envAddress("TOKEN_ADDRESS");

        vm.startBroadcast(deployerPrivateKey);

        escrow = new Escrow(IERC20(token));

        vm.stopBroadcast();

        console2.log("Escrow deployed at:", address(escrow));
        console2.log("Token:", token);
    }
}
