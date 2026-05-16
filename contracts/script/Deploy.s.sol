// SPDX-License-Identifier: UNLICENSED
pragma solidity 0.8.33;

import {Script, console2} from "forge-std/Script.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

import {Escrow} from "../src/Escrow.sol";
import {NettingSettlement} from "../src/NettingSettlement.sol";
import {RiveUSD} from "../src/mocks/RiveUSD.sol";

contract DeployScript is Script {
    RiveUSD public rusd;
    Escrow public escrow;
    NettingSettlement public nettingSettlement;

    function run() public {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address settler = vm.envOr("NETTING_SETTLER_ADDRESS", vm.addr(deployerPrivateKey));

        vm.startBroadcast(deployerPrivateKey);

        rusd = new RiveUSD();
        escrow = new Escrow(IERC20(address(rusd)));
        nettingSettlement = new NettingSettlement(IERC20(address(rusd)), settler);

        vm.stopBroadcast();

        console2.log("RiveUSD deployed at:", address(rusd));
        console2.log("Escrow deployed at:", address(escrow));
        console2.log("NettingSettlement deployed at:", address(nettingSettlement));
        console2.log("Netting settler:", settler);
    }
}
