#!/bin/bash
sed -i '295s/.*/\tfmt.Println("\\n" + strings.Repeat("=", 60))/' test_race.go
sed -i '297s/.*/\tfmt.Println(strings.Repeat("=", 60))/' test_race.go
