\ Echoes keys on the grid. ENTER ends a line and prints its length, an
\ empty line ends the program.
VARIABLE N
: LINE ( -- n ) 0 N ! BEGIN KEY DUP 10 = IF DROP N @ EXIT THEN EMIT 1 N +! AGAIN ;
: ECHO ( -- ) BEGIN ." > " LINE DUP 0 > WHILE CR ." length " . CR REPEAT DROP ." done" CR ;
ECHO
