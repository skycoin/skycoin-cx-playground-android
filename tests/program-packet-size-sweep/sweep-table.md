| Operation | 1400 | 1200 | 1024 | 512 | 256 | 128 | 64 | 32 | 16 | Floor | Below the floor |
|---|---|---|---|---|---|---|---|---|---|---|---|
| list manifests 0x01 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | fails | fails | 42 | reply: header 6, 32-byte address, length 4 |
| list manifests prefix 0x05 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/23 | 15 | reply: header 6, prefix 1+4, length 4 |
| get manifest 0x02 | 4/2586 | 6/2676 | 6/2676 | 12/2946 | 24/3486 | 60/5106 | 268/14466 | fails | fails | 47 | request 44 with the 32-byte address, reply 46 before the first byte |
| get manifest by prefix 0x06 | 4/2494 | 6/2538 | 6/2538 | 10/2626 | 22/2890 | 46/3418 | 118/5002 | 536/14198 | fails | 20 | reply: header 6, prefix 1+4, offset 4, length 4, one byte |
| get block list 0x03 | 2/1048 | 2/1048 | 2/1048 | 6/1224 | 10/1400 | 40/2720 | fails | fails | fails | 92 | reply 44 with the 32-byte address, plus one 48-byte block entry |
| get block list bytes 0x07 | 2/1004 | 2/1004 | 2/1004 | 4/1048 | 10/1180 | 20/1400 | 48/2016 | 214/5668 | fails | 20 | as get manifest by prefix |
| get blocks 0x04 | 16/9014 | 18/9110 | 26/9494 | fails | fails | fails | fails | fails | fails | 560 | reply 48 with the 32-byte address, plus one whole 512-byte block |
| get block part 0x08 | 40/9186 | 40/9186 | 40/9186 | 64/9750 | 96/10502 | 166/12147 | 392/17458 | 1294/38655 | 16492/263872 | 16 | reply: header 6, block prefix 1+4, offset 2, length 2, one byte |
| fetch program, forms=full | 26/12576 | 28/12672 | 38/13152 | fails | fails | fails | fails | fails | fails | 560 | get blocks |
| fetch program, forms=short | 54/12627 | 54/12627 | 54/12627 | 88/13423 | 134/14498 | 230/16745 | 542/24044 | 1836/54261 | fails | 20 | get manifest by prefix |
| subscribe 0x11 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 10 | request and reply: header 6, lease 4 |
| have 0x10 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | fails | fails | 50 | request: header 6, 32-byte address, lengths 4 and 8 |
| announce and request one program, forms=full | 14/11236 | 16/11330 | 20/11522 | fails | fails | fails | fails | fails | fails | 560 | get blocks |
| announce and request one program, forms=short | 26/11186 | 27/11209 | 27/11209 | 41/11483 | 63/11925 | 110/12866 | 259/15841 | fails | fails | 50 | the have |
| get peers 0x12 | 2/138 | 2/138 | 2/138 | 2/138 | 2/138 | 2/113 | 2/63 | 2/38 | fails | 32 | reply: header 6, count 1, one 25-byte IPv6 entry |
