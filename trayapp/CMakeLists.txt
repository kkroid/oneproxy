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
    instance_guard.cpp
    instance_guard.h
    platform/platform.h
    ${ONEPROXY_PLATFORM_SOURCE}
    resources/icons.qrc
)

# Keep the executable in the build root so the artifact sync target has no
# target-file generator expression (which would create a dependency cycle).
set_target_properties(oneproxy-tray PROPERTIES
    RUNTIME_OUTPUT_DIRECTORY "${CMAKE_CURRENT_BINARY_DIR}"
    LIBRARY_OUTPUT_DIRECTORY "${CMAKE_CURRENT_BINARY_DIR}"
    ARCHIVE_OUTPUT_DIRECTORY "${CMAKE_CURRENT_BINARY_DIR}")
foreach(cfg Debug Release RelWithDebInfo MinSizeRel)
    set_target_properties(oneproxy-tray PROPERTIES
        RUNTIME_OUTPUT_DIRECTORY_${cfg} "${CMAKE_CURRENT_BINARY_DIR}"
        LIBRARY_OUTPUT_DIRECTORY_${cfg} "${CMAKE_CURRENT_BINARY_DIR}"
        ARCHIVE_OUTPUT_DIRECTORY_${cfg} "${CMAKE_CURRENT_BINARY_DIR}")
endforeach()

if(WIN32)
    set_target_properties(oneproxy-tray PROPERTIES WIN32_EXECUTABLE TRUE)
elseif(APPLE)
    set_target_properties(oneproxy-tray PROPERTIES MACOSX_BUNDLE TRUE)
endif()

target_link_libraries(oneproxy-tray PRIVATE Qt6::Widgets)

set(ONEPROXY_LIBRARY_PATH
    "${CMAKE_SOURCE_DIR}/../${ONEPROXY_LIBRARY_NAME}"
    CACHE FILEPATH "Path to the OneProxy shared library")

set(ONEPROXY_CLI_PATH "${CMAKE_SOURCE_DIR}/../oneproxy${CMAKE_EXECUTABLE_SUFFIX}"
    CACHE FILEPATH "Path to the OneProxy CLI executable")

if(APPLE)
    set(ONEPROXY_RUNTIME_DIR "${CMAKE_CURRENT_BINARY_DIR}/oneproxy-tray.app/Contents/MacOS")
else()
    set(ONEPROXY_RUNTIME_DIR "${CMAKE_CURRENT_BINARY_DIR}")
endif()

if(EXISTS "${ONEPROXY_LIBRARY_PATH}")
    add_custom_target(oneproxy-sync ALL
        COMMAND ${CMAKE_COMMAND} -E make_directory "${ONEPROXY_RUNTIME_DIR}"
        COMMAND ${CMAKE_COMMAND} -E copy_if_different
            "${ONEPROXY_LIBRARY_PATH}"
            "${ONEPROXY_RUNTIME_DIR}/${ONEPROXY_LIBRARY_NAME}"
        COMMAND ${CMAKE_COMMAND} -E compare_files
            "${ONEPROXY_LIBRARY_PATH}"
            "${ONEPROXY_RUNTIME_DIR}/${ONEPROXY_LIBRARY_NAME}"
        COMMAND ${CMAKE_COMMAND} -E copy_if_different
            "${ONEPROXY_CLI_PATH}"
            "${ONEPROXY_RUNTIME_DIR}/oneproxy${CMAKE_EXECUTABLE_SUFFIX}"
        COMMAND ${CMAKE_COMMAND} -E compare_files
            "${ONEPROXY_CLI_PATH}"
            "${ONEPROXY_RUNTIME_DIR}/oneproxy${CMAKE_EXECUTABLE_SUFFIX}"
    )
    add_dependencies(oneproxy-tray oneproxy-sync)
endif()

if(BUILD_TESTING)
    add_subdirectory(tests)
endif()
