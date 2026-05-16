# Work Order Lifecycle

Dokumen ini berisi alur end-to-end untuk membuat work order, funding escrow, submit delivery, sampai release payment. Command memakai `curl` untuk backend dan `cast` Foundry untuk kontrak `RiveUSD` serta `Escrow`.

## Prasyarat

Payer dan payee harus sudah terdaftar sebagai `agents` di backend. Endpoint `POST /api/work-orders` akan memvalidasi `parties.payer` dan `parties.payee` terhadap wallet agent yang ada di database.

Pastikan `cast`, `curl`, dan `jq` tersedia, lalu set environment berikut:

```sh
export API_BASE_URL="http://localhost:8080/api"
export RPC_URL="<0G_EVM_RPC_URL>"

export PAYER_PRIVATE_KEY="0x..."
export PAYEE_PRIVATE_KEY="0x..."

export PAYER="$(cast wallet address --private-key "$PAYER_PRIVATE_KEY")"
export PAYEE="$(cast wallet address --private-key "$PAYEE_PRIVATE_KEY")"

export AMOUNT="10000000000000000000"
export RUSD_ADDRESS="0xB053E106D5236e4c4cD1b7DA0aC51bA0B318C7a0"
export ESCROW_ADDRESS="0xe3de5a57b960aeaa4d1d01b46665599067476b6d"
```

Catatan: `RiveUSD` memakai 18 decimals, jadi contoh `AMOUNT` di atas adalah `10 rUSD`.

Catatan operasional: kontrak `Escrow` mulai dari `nextOrderID = 0`, tetapi backend saat ini memvalidasi `onchainOrderID` sebagai bilangan positif. Untuk demo backend delivery, gunakan on-chain order ID `>= 1` atau perbaiki validasi backend di task terpisah.

## 1. Payer Membuat Work Order di Backend

Backend akan menyimpan work order draft, upload spec ke 0G Storage, lalu mengembalikan `root_hash`. Nilai `root_hash` ini dipakai sebagai `specHash` saat membuat order on-chain.

```sh
cat > work-order-request.json <<EOF
{
  "idempotency_key": "wo-demo-$(date +%s)",
  "parties": {
    "payer": "$PAYER",
    "payee": "$PAYEE"
  },
  "task": {
    "title": "Generate cleaned review dataset",
    "description": "Payee membuat file JSON berisi hasil kerja sesuai acceptance criteria.",
    "category": "data-processing"
  },
  "deliverable": {
    "format": "json",
    "submission": {
      "method": "rive-storage",
      "endpoint": "/api/storage/upload"
    }
  },
  "acceptanceCriteria": [
    {
      "id": "ac1",
      "description": "Delivery berupa JSON valid dan root hash-nya disubmit oleh payee."
    }
  ],
  "compensation": {
    "amount": "$AMOUNT",
    "asset": "rUSD",
    "chain": "0g-mainnet"
  },
  "deadline": "2026-12-31T23:59:59Z"
}
EOF

curl -sS -X POST "$API_BASE_URL/work-orders" \
  -H "Content-Type: application/json" \
  --data @work-order-request.json \
  | tee work-order-response.json

export SPEC_HASH="$(jq -r '.data.root_hash' work-order-response.json)"
echo "$SPEC_HASH"
```

Response sukses berbentuk envelope:

```json
{
  "success": true,
  "data": {
    "id": "...",
    "root_hash": "0x...",
    "tx_hash": "0x..."
  }
}
```

## 2. Payer Mint Mock rUSD

`RiveUSD` adalah mock token dengan fungsi `mint(address,uint256)` publik.

```sh
cast send "$RUSD_ADDRESS" \
  "mint(address,uint256)" \
  "$PAYER" \
  "$AMOUNT" \
  --rpc-url "$RPC_URL" \
  --private-key "$PAYER_PRIVATE_KEY"
```

Cek balance payer:

```sh
cast call "$RUSD_ADDRESS" \
  "balanceOf(address)(uint256)" \
  "$PAYER" \
  --rpc-url "$RPC_URL"
```

## 3. Payer Approve rUSD untuk Escrow

Escrow akan memanggil `transferFrom` saat `createOrder`, jadi payer harus memberi allowance minimal sebesar `AMOUNT`.

```sh
cast send "$RUSD_ADDRESS" \
  "approve(address,uint256)" \
  "$ESCROW_ADDRESS" \
  "$AMOUNT" \
  --rpc-url "$RPC_URL" \
  --private-key "$PAYER_PRIVATE_KEY"
```

Cek allowance:

```sh
cast call "$RUSD_ADDRESS" \
  "allowance(address,address)(uint256)" \
  "$PAYER" \
  "$ESCROW_ADDRESS" \
  --rpc-url "$RPC_URL"
```

## 4. Payer Create Order di Escrow

Baca `nextOrderID` sebelum transaksi. Nilai ini adalah `ONCHAIN_ORDER_ID` yang akan dipakai untuk delivery dan release.

```sh
export ONCHAIN_ORDER_ID="$(cast call "$ESCROW_ADDRESS" \
  "nextOrderID()(uint256)" \
  --rpc-url "$RPC_URL")"

echo "$ONCHAIN_ORDER_ID"
```

Create order on-chain:

```sh
cast send "$ESCROW_ADDRESS" \
  "createOrder(address,uint256,bytes32)" \
  "$PAYEE" \
  "$AMOUNT" \
  "$SPEC_HASH" \
  --rpc-url "$RPC_URL" \
  --private-key "$PAYER_PRIVATE_KEY"
```

Cek data order di kontrak:

```sh
cast call "$ESCROW_ADDRESS" \
  "orders(uint256)(address,uint64,uint8,address,uint256,bytes32)" \
  "$ONCHAIN_ORDER_ID" \
  --rpc-url "$RPC_URL"
```

Setelah webhook QuickNode memproses event `OrderCreated`, backend akan mengubah work order dari `draft` menjadi `funded` dan menyimpan `onchain_order_id`.

```sh
curl -sS "$API_BASE_URL/work-orders/$ONCHAIN_ORDER_ID" | jq
```

## 5. Payee Upload Hasil Kerja JSON

Upload delivery sebagai JSON ke 0G Storage melalui backend. Backend akan canonicalize JSON sebelum upload.

```sh
cat > delivery.json <<EOF
{
  "workOrderID": "$ONCHAIN_ORDER_ID",
  "result": {
    "status": "completed",
    "items": [
      {
        "id": "item-1",
        "value": "sample output"
      }
    ]
  }
}
EOF

curl -sS -X POST "$API_BASE_URL/storage/upload" \
  -H "Content-Type: application/json" \
  --data @delivery.json \
  | tee delivery-upload-response.json

export DELIVERY_HASH="$(jq -r '.data.root_hash' delivery-upload-response.json)"
echo "$DELIVERY_HASH"
```

## 6. Payee Sign Delivery Hash

Backend memverifikasi signature dari message berikut:

```text
deliver:{onchainOrderID}:{deliveryHash}
```

Format ini mengikuti `backend/internal/usecase/work_order_usecase.go`:

```go
message := fmt.Sprintf("deliver:%s:%s", normalizedOrderID, request.DeliveryHash)
```

`recoverEthereumAddress` memakai Ethereum signed message prefix melalui `accounts.TextHash`, sehingga sign dengan `cast wallet sign` tanpa pre-hash manual.

```sh
export DELIVERY_MESSAGE="deliver:${ONCHAIN_ORDER_ID}:${DELIVERY_HASH}"

export DELIVERY_SIGNATURE="$(cast wallet sign \
  --private-key "$PAYEE_PRIVATE_KEY" \
  "$DELIVERY_MESSAGE")"

echo "$DELIVERY_SIGNATURE"
```

## 7. Payee Submit Delivery ke Backend

Submit `deliveryHash` dan `signature` ke work order funded. Signature harus berasal dari private key payee yang terdaftar untuk order tersebut.

```sh
jq -n \
  --arg deliveryHash "$DELIVERY_HASH" \
  --arg signature "$DELIVERY_SIGNATURE" \
  '{
    deliveryHash: $deliveryHash,
    signature: $signature
  }' > submit-delivery-request.json

curl -sS -X POST "$API_BASE_URL/work-orders/$ONCHAIN_ORDER_ID/delivery" \
  -H "Content-Type: application/json" \
  --data @submit-delivery-request.json \
  | tee submit-delivery-response.json
```

Response sukses:

```json
{
  "success": true,
  "data": {
    "onchain_order_id": "1",
    "deliverable_cid": "0x...",
    "delivered_at": "..."
  }
}
```

## 8. Payer Release Order di Escrow

Setelah payer menerima dan menyetujui delivery, payer merilis dana escrow ke payee.

```sh
cast send "$ESCROW_ADDRESS" \
  "releaseOrder(uint256)" \
  "$ONCHAIN_ORDER_ID" \
  --rpc-url "$RPC_URL" \
  --private-key "$PAYER_PRIVATE_KEY"
```

Cek balance payee:

```sh
cast call "$RUSD_ADDRESS" \
  "balanceOf(address)(uint256)" \
  "$PAYEE" \
  --rpc-url "$RPC_URL"
```

Cek state order di kontrak. `state = 2` berarti `Released`.

```sh
cast call "$ESCROW_ADDRESS" \
  "orders(uint256)(address,uint64,uint8,address,uint256,bytes32)" \
  "$ONCHAIN_ORDER_ID" \
  --rpc-url "$RPC_URL"
```

Setelah webhook QuickNode memproses event `OrderReleased`, backend akan mengubah status work order menjadi `completed`.

```sh
curl -sS "$API_BASE_URL/work-orders/$ONCHAIN_ORDER_ID" | jq
```

## Ringkasan State

| Tahap | Sistem | State |
| --- | --- | --- |
| `POST /work-orders` | Backend | `draft` |
| `createOrder` + webhook `OrderCreated` | Backend | `funded` |
| `POST /work-orders/{onchainOrderID}/delivery` | Backend | delivery hash tersimpan |
| `releaseOrder` + webhook `OrderReleased` | Backend | `completed` |
| `releaseOrder` | Escrow | `Released` |

