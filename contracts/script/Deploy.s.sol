// SPDX-License-Identifier: UNLICENSED
pragma solidity 0.8.33;

import {Script, console2} from "forge-std/Script.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

import {Escrow} from "../src/Escrow.sol";
import {RiveUSD} from "../src/mocks/RiveUSD.sol";

contract DeployScript is Script {
    RiveUSD public rusd;
    Escrow public escrow;

    function run() public {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");

        vm.startBroadcast(deployerPrivateKey);

        rusd = new RiveUSD();
        escrow = new Escrow(IERC20(address(rusd)));

        vm.stopBroadcast();

        console2.log("RiveUSD deployed at:", address(rusd));
        console2.log("Escrow deployed at:", address(escrow));
    }
}
