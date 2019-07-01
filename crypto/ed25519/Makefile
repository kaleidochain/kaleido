
lib/libsodium.a:
	cd libsodium && ./autogen.sh && \
		./configure --disable-shared --prefix="$(shell pwd)/" && \
		$(MAKE) && $(MAKE) install && $(MAKE) distclean

clean:
	cd libsodium && test ! -e Makefile || make clean
	rm -rf lib

