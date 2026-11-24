| Operation | 1400 | 1200 | 1024 | 512 | 256 | 128 | 64 | 32 | 16 | Floor | Below the floor |
|---|---|---|---|---|---|---|---|---|---|---|---|
| list manifests 0x01 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | fails | fails | 42 | reply: header 6, 32-byte address, length 4 |
| list manifests prefix 0x05 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/23 | 15 | reply: header 6, prefix 1+4, length 4 |
| get manifest 0x02 | 4/2652 | 6/2742 | 6/2742 | 12/3012 | 24/3552 | 62/5262 | 276/14892 | fails | fails | 47 | request 44 with the 32-byte address, reply 46 before the first byte |
| get manifest by prefix 0x06 | 4/2560 | 6/2604 | 6/2604 | 12/2736 | 22/2956 | 48/3528 | 122/5156 | 550/14572 | fails | 20 | reply: header 6, prefix 1+4, offset 4, length 4, one byte |
| get block list 0x03 | 2/1096 | 2/1096 | 4/1184 | 6/1272 | 12/1536 | 42/2856 | fails | fails | fails | 92 | reply 44 with the 32-byte address, plus one 48-byte block entry |
| get block list bytes 0x07 | 2/1052 | 2/1052 | 4/1096 | 6/1140 | 10/1228 | 20/1448 | 50/2108 | 224/5936 | fails | 20 | as get manifest by prefix |
| get blocks 0x04 | 16/9367 | 18/9463 | 28/9943 | 68/11863 | 102/13495 | 238/20023 | 1082/60535 | fails | fails | 49 | request and reply 48 with the 32-byte address, one byte |
| get block part 0x08 | 42/9586 | 42/9586 | 42/9586 | 68/10197 | 102/10996 | 174/12688 | 408/18187 | 1350/40324 | 17198/275168 | 16 | reply: header 6, block prefix 1+4, offset 2, length 2, one byte |
| fetch program, forms by size | 26/12995 | 28/13091 | 40/13667 | 92/13936 | 140/15058 | 238/17352 | 562/24933 | 1902/56231 | fails | 20 | get manifest by prefix |
| subscribe 0x11 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 10 | request and reply: header 6, lease 4 |
| have 0x10 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | fails | fails | 50 | request: header 6, 32-byte address, lengths 4 and 8 |
| flood one program, forms by size | 14/11655 | 16/11749 | 21/11989 | 44/11963 | 66/12401 | 115/13384 | 269/16458 | fails | fails | 50 | the have |
| get peers 0x12 | 2/228 | 2/228 | 2/228 | 2/228 | 2/228 | 4/247 | fails | fails | fails | 110 | reply 11 plus the longest peer entry, 99 |
| get peer part 0x13 | 10/314 | 10/314 | 10/314 | 10/314 | 10/314 | 10/314 | 12/335 | 26/482 | 108/1343 | 13 | reply: header 6, total 2, index 2, length 1, offset 1, one byte |
| get peers, forms by size | 2/228 | 2/228 | 2/228 | 2/228 | 2/228 | 4/247 | 12/327 | 36/577 | 118/1438 | 13 | get peer part |
