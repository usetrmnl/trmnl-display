OPSYS:=$(shell uname)
ifeq ($(OPSYS), Linux)
LIBS=-g -lFastEPD -ltrmnl -lbb_epaper -lgpiod -lpngdec -lJPEGDEC -lbb_scd41 -lbb_temperature -lcurl $(shell pkg-config sdl2 --libs)
else
LIBS=-g -ltrmnl -lpngdec -lJPEGDEC -L/opt/homebrew/opt/curl/lib -lcurl $(shell pkg-config sdl2 --libs)
endif
CFLAGS=-ggdb -D__LINUX__ -Wall -O2 -I../../ $(shell pkg-config sdl2 --cflags)

trmnl_display: main.o cJSON.o cJSON_Utils.o
	$(CXX) main.o cJSON.o cJSON_Utils.o $(LIBS) -o trmnl_display
	sudo cp trmnl_display /usr/local/bin

cJSON.o: cJSON.h cJSON.c
	$(CC) $(CFLAGS) -c cJSON.c

cJSON_Utils.o: cJSON_Utils.h cJSON_Utils.c
	$(CC) $(CFLAGS) -c cJSON_Utils.c

main.o: main.cpp
	$(CXX) $(CFLAGS) -c main.cpp

clean:
	rm -rf *.o trmnl_display
