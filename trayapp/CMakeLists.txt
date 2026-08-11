cmake_minimum_required(VERSION 3.16)
project(OneProxyTray LANGUAGES CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTORCC ON)

include(CTest)
find_package(Qt6 REQUIRED COMPONENTS Widgets)
if(BUILD_TESTING)
    find_package(Qt6 REQUIRED COMPONENTS Test)
endif()

if(WIN32)
    set(ONEPROXY_PLATFORM_SOURCE platform/platform_windows.cpp)
    set(ONEPROXY_LIBRARY_NAME oneproxy.dll)
elseif(APPLE)
    set(ONEPROXY_PLATFORM_SOURCE platform/platform_unix.cpp)
    set(ONEPROXY_LIBRARY_NAME liboneproxy.dylib)
else()
    set(ONEPROXY_PLATFORM_SOURCE platform/platform_unix.cpp)
    set(ONEPROXY_LIBRARY_NAME liboneproxy.so)
endif()

add_executable(oneproxy-tray
    main.cpp
    platform/platform.h
    ${ONEPROXY_PLATFORM_SOURCE}
    resources/icons.qrc
)

if(WIN32)
    set_target_properties(oneproxy-tray PROPERTIES WIN32_EXECUTABLE TRUE)
elseif(APPLE)
    set_target_properties(oneproxy-tray PROPERTIES MACOSX_BUNDLE TRUE)
endif()

target_link_libraries(oneproxy-tray PRIVATE Qt6::Widgets)

set(ONEPROXY_LIBRARY_PATH
    "${CMAKE_SOURCE_DIR}/../${ONEPROXY_LIBRARY_NAME}"
    CACHE FILEPATH "Path to the OneProxy shared library")

if(EXISTS "${ONEPROXY_LIBRARY_PATH}")
    add_custom_command(TARGET oneproxy-tray POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy_if_different
            "${ONEPROXY_LIBRARY_PATH}"
            "$<TARGET_FILE_DIR:oneproxy-tray>/${ONEPROXY_LIBRARY_NAME}"
    )
endif()

if(BUILD_TESTING)
    add_subdirectory(tests)
endif()
