HOST:=$(shell uname)
ifeq ($(HOST), Darwin)
LIBS=-g -lpngdec -lJPEGDEC
else
LIBS=-g -lbb_epaper -lpngdec -lJPEGDEC -lgpiod
endif
CFLAGS=-ggdb -D__LINUX__ -Wall -O2 -I../../

all: show_img

sdl: CFLAGS+=-D__SDL__ $(shell pkg-config sdl2 --cflags)

sdl: LIBS+=$(shell pkg-config sdl2 --libs) 

sdl: all

show_img: main.o cJSON.o cJSON_Utils.o
	$(CXX) main.o cJSON.o cJSON_Utils.o $(LIBS) -o show_img ;\
	sudo mkdir -p /usr/local/bin
	sudo cp show_img /usr/local/bin

cJSON.o: cJSON.h cJSON.c
	$(CC) $(CFLAGS) -c cJSON.c

cJSON_Utils.o: cJSON_Utils.h cJSON_Utils.c
	$(CC) $(CFLAGS) -c cJSON_Utils.c

main.o: main.cpp
	$(CXX) $(CFLAGS) -c main.cpp

clean:
	rm -rf *.o show_img
