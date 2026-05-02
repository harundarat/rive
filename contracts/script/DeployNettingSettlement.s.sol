// SPDX-License-Identifier: UNLICENSED
pragma solidity 0.8.33;

import {Script, console2} from "forge-std/Script.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

import {NettingSettlement} from "../src/NettingSettlement.sol";

contract DeployNettingSettlementScript is Script {
    NettingSettlement public nettingSettlement;

    function run() public {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address token = vm.envAddress("TOKEN_ADDRESS");
        address settler = vm.envOr("NETTING_SETTLER_ADDRESS", vm.addr(deployerPrivateKey));

        vm.startBroadcast(deployerPrivateKey);

        nettingSettlement = new NettingSettlement(IERC20(token), settler);

        vm.stopBroadcast();

        console2.log("NettingSettlement deployed at:", address(nettingSettlement));
        console2.log("Token:", token);
        console2.log("Netting settler:", settler);
    }
}
