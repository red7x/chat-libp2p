.PHONY: test
test:
	# CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -o test.exe chat-libp2p/cmd/test
	# mv ./test.exe /mnt/c/Users/Administrator/
	go build -o test chat-libp2p/cmd/test

.PHONY: server
server:
	go build -o server -ldflags=-checklinkname=0 chat-libp2p/cmd/server

.PHONY: mobile-android
mobile-android:
	# go install golang.org/x/mobile/cmd/gomobile@latest
	# https://developer.android.google.cn/ndk/downloads?hl=zh-cn download ndk
	# unzip and move it to ~/Android/Sdk/ndk then copy meta and toolchains directory into the wrap.sh directory
	# https://developer.android.com/studio?hl=zh-cn download commandlinetools
	# unzip and move it to ~/Android/Sdk then exec ./cmdline-tools/bin/sdkmanager --sdk_root=. --licenses
	# then exec ./cmdline-tools/bin/sdkmanager --sdk_root=. "platforms;android-34" "tools" "platform-tools" "extras;google;m2repository"
	go get -u golang.org/x/mobile/bind
	gomobile bind -v -target=android/arm,android/arm64 --androidapi 21 -o mobile.aar -ldflags=-checklinkname=0 chat-libp2p/cmd/client/mobile

.PHONY: mobile-android-debug
mobile-android-debug:
	go get -u golang.org/x/mobile/bind
	gomobile bind -v -target=android/arm,android/arm64 --androidapi 21 -o mobile.aar -ldflags="-checklinkname=0 -X mobile.DebugMode=true" chat-libp2p/cmd/client/mobile

.PHONY: pc-windows
pc-windows:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -o pc.exe chat-libp2p/cmd/client/pc

.PHONY: pc-windows-debug
pc-windows-debug:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -o pc.exe chat-libp2p/cmd/client/pc