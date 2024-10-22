cmake_minimum_required(VERSION 3.5.0)
project(snapd-installation-monitor)

set(CMAKE_INCLUDE_CURRENT_DIR ON)
set(CMAKE_AUTOMOC ON)
find_package(Qt6 COMPONENTS Widgets DBus REQUIRED)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_executable(snapd-installation-monitor main.cpp)
target_link_libraries(snapd-installation-monitor Qt6::Widgets Qt6::DBus)
