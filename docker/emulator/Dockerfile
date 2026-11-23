# A headless Android emulator for the CX playground test rig.
#
# The build stage needs Java for sdkmanager and avdmanager. The final image
# holds only the emulator, adb, one system image and one virtual device.

ARG DEBIAN=debian:trixie-slim

FROM ${DEBIAN} AS sdk
ARG CMDLINE_TOOLS=16111833
ARG CMDLINE_TOOLS_SHA1=e025545c62a8e64c7559119566a569fb1dec5f60
ARG API=35
ENV ANDROID_SDK_ROOT=/opt/android-sdk
RUN apt-get update \
 && apt-get install -y --no-install-recommends openjdk-21-jre-headless curl unzip ca-certificates \
 && rm -rf /var/lib/apt/lists/*
RUN curl -fsSLo /tmp/tools.zip https://dl.google.com/android/repository/commandlinetools-linux-${CMDLINE_TOOLS}_latest.zip \
 && echo "${CMDLINE_TOOLS_SHA1}  /tmp/tools.zip" | sha1sum -c - \
 && mkdir -p ${ANDROID_SDK_ROOT}/cmdline-tools \
 && unzip -q /tmp/tools.zip -d ${ANDROID_SDK_ROOT}/cmdline-tools \
 && mv ${ANDROID_SDK_ROOT}/cmdline-tools/cmdline-tools ${ANDROID_SDK_ROOT}/cmdline-tools/latest \
 && rm /tmp/tools.zip
ENV PATH=${ANDROID_SDK_ROOT}/cmdline-tools/latest/bin:${PATH}
RUN yes | sdkmanager --licenses > /dev/null \
 && sdkmanager --install emulator platform-tools "system-images;android-${API};default;x86_64" > /dev/null
RUN echo no | avdmanager create avd -n rig -k "system-images;android-${API};default;x86_64" \
 && printf '%s\n' \
      hw.ramSize=2048 \
      vm.heapSize=256 \
      disk.dataPartition.size=2G \
      hw.lcd.width=720 \
      hw.lcd.height=1280 \
      hw.lcd.density=320 \
      hw.keyboard=yes \
      hw.audioInput=no \
      hw.audioOutput=no \
      hw.camera.back=none \
      hw.camera.front=none \
      hw.gpu.enabled=yes \
      hw.gpu.mode=swiftshader_indirect \
      fastboot.forceColdBoot=yes \
    >> /root/.android/avd/rig.avd/config.ini

FROM ${DEBIAN}
ENV ANDROID_SDK_ROOT=/opt/android-sdk \
    ANDROID_HOME=/opt/android-sdk \
    ANDROID_AVD_HOME=/root/.android/avd
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      libpulse0 libx11-6 libx11-xcb1 libxcb1 libdbus-1-3 libgl1 libnss3 libxkbfile1 \
      libxcomposite1 libxcursor1 libxi6 libxtst6 libxdamage1 libxrandr2 libfontconfig1 \
      libtcmalloc-minimal4t64 socat iproute2 tcpdump \
 && rm -rf /var/lib/apt/lists/*
COPY --from=sdk /opt/android-sdk/emulator /opt/android-sdk/emulator
COPY --from=sdk /opt/android-sdk/platform-tools /opt/android-sdk/platform-tools
COPY --from=sdk /opt/android-sdk/system-images /opt/android-sdk/system-images
COPY --from=sdk /opt/android-sdk/licenses /opt/android-sdk/licenses
COPY --from=sdk /root/.android /root/.android
COPY entrypoint.sh /entrypoint.sh
ENV PATH=/opt/android-sdk/emulator:/opt/android-sdk/platform-tools:${PATH}
# 15555 forwards to the emulator's adb port, 15554 to its console.
EXPOSE 15555 15554
ENTRYPOINT ["/entrypoint.sh"]
