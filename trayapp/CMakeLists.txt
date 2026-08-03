cmake_minimum_required(VERSION 3.16)
project(OneProxyTray LANGUAGES CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

find_package(Qt6 REQUIRED COMPONENTS Widgets)

add_executable(oneproxy-tray WIN32
    main.cpp
)

target_link_libraries(oneproxy-tray PRIVATE Qt6::Widgets)

# Copy DLL next to exe after build
add_custom_command(TARGET oneproxy-tray POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy_if_different
        "${CMAKE_SOURCE_DIR}/../oneproxy.dll"
        "$<TARGET_FILE_DIR:oneproxy-tray>/oneproxy.dll"
)
